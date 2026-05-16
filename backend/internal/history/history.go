package history

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type API struct{ db *sql.DB }

func New(db *sql.DB) *API { return &API{db: db} }

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
		http.Error(w, "bad json", 400)
		return
	}
	if p.StartedAt == 0 {
		p.StartedAt = time.Now().Unix()
	}
	if p.Source == "" {
		p.Source = "local"
	}
	completed := 0
	if p.Completed {
		completed = 1
	}
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO plays(track_id,started_at,duration_played_sec,completed,source) VALUES(?,?,?,?,?)`,
		p.TrackID, p.StartedAt, p.DurationPlayedSec, completed, p.Source)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (a *API) recent(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT p.id, p.track_id, t.title, IFNULL(t.artist,''), IFNULL(t.album,''), p.started_at, p.duration_played_sec, p.completed, p.source
		FROM plays p JOIN tracks t ON t.id=p.track_id
		ORDER BY p.started_at DESC LIMIT 50`)
	if err != nil {
		http.Error(w, err.Error(), 500)
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
		rows.Scan(&x.ID, &x.TrackID, &x.Title, &x.Artist, &x.Album, &x.StartedAt, &x.DurationPlayedSec, &c, &x.Source)
		x.Completed = c == 1
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
