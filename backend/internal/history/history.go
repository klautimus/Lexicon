package history

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/auth"
)

type API struct{ db *sql.DB }

func New(db *sql.DB) *API { return &API{db: db} }

func getUserID(r *http.Request) int64 {
	if u, ok := auth.UserFromContext(r.Context()); ok {
		return u.UserID
	}
	return 0
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[history] writeJSON encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("[history] writeError encode: %v", err)
	}
}

func (a *API) Mount(r chi.Router) {
	r.Post("/api/history/play", a.recordPlay)
	r.Get("/api/history/recent", a.recent)
}

type playReq struct {
	TrackID           int64  `json:"track_id"`
	DurationPlayedSec int    `json:"duration_played_sec"`
	Completed         bool   `json:"completed"`
	Source            string `json:"source"`
	StartedAt         int64  `json:"started_at"`
}

func (a *API) recordPlay(w http.ResponseWriter, r *http.Request) {
	var p playReq
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		log.Printf("[history] recordPlay decode: %v", err)
		writeError(w, "bad json", 400)
		return
	}
	if p.StartedAt == 0 {
		p.StartedAt = time.Now().Unix()
	}
	if p.Source == "" {
		p.Source = "local"
	}
	// Validate track exists before recording play
	var exists int
	if err := a.db.QueryRowContext(r.Context(), `SELECT 1 FROM tracks WHERE id=?`, p.TrackID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, "track not found", 404)
			return
		}
		log.Printf("[history] recordPlay track lookup track %d: %v", p.TrackID, err)
		writeError(w, err.Error(), 500)
		return
	}
	completed := 0
	if p.Completed {
		completed = 1
	}
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO plays(track_id,started_at,duration_played_sec,completed,source,user_id) VALUES(?,?,?,?,?,?)`,
		p.TrackID, p.StartedAt, p.DurationPlayedSec, completed, p.Source, nil)
	if err != nil {
		log.Printf("[history] recordPlay insert track %d: %v", p.TrackID, err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) recent(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT p.id, p.track_id, IFNULL(t.title,'(deleted)'), IFNULL(t.artist,''), IFNULL(t.album,''), p.started_at, p.duration_played_sec, p.completed, p.source
		FROM plays p LEFT JOIN tracks t ON t.id=p.track_id
		WHERE p.user_id IS NULL OR p.user_id = ?
		ORDER BY p.started_at DESC LIMIT 50`, getUserID(r))
	if err != nil {
		log.Printf("[history] recent query: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Row struct {
		ID                int64  `json:"id"`
		TrackID           int64  `json:"track_id"`
		Title             string `json:"title"`
		Artist            string `json:"artist"`
		Album             string `json:"album"`
		StartedAt         int64  `json:"started_at"`
		DurationPlayedSec int    `json:"duration_played_sec"`
		Completed         bool   `json:"completed"`
		Source            string `json:"source"`
	}
	out := []Row{}
	for rows.Next() {
		var x Row
		var c int
		if err := rows.Scan(&x.ID, &x.TrackID, &x.Title, &x.Artist, &x.Album, &x.StartedAt, &x.DurationPlayedSec, &c, &x.Source); err != nil {
			log.Printf("[history] recent scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		x.Completed = c == 1
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[history] recent rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}
