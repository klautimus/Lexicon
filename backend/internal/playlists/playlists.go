package playlists

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/models"
)

type Playlist struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	TrackCount    int    `json:"track_count"`
	TotalDuration int    `json:"total_duration"`
	CreatedAt     int64  `json:"created_at"`
}

type PlaylistWithTracks struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	TrackCount    int            `json:"track_count"`
	TotalDuration int            `json:"total_duration"`
	CreatedAt     int64          `json:"created_at"`
	Tracks        []models.Track `json:"tracks"`
}

type API struct{ db *sql.DB }

func New(db *sql.DB) *API { return &API{db: db} }

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/playlists", a.list)
	r.Post("/api/playlists", a.create)
	r.Get("/api/playlists/{id}", a.get)
	r.Put("/api/playlists/{id}", a.update)
	r.Delete("/api/playlists/{id}", a.delete)
	r.Post("/api/playlists/{id}/tracks", a.addTrack)
	r.Delete("/api/playlists/{id}/tracks/{position}", a.removeTrack)
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT p.id, p.name, COUNT(i.track_id), COALESCE(SUM(t.duration_sec),0), p.created_at
		FROM playlists p
		LEFT JOIN playlist_items i ON i.playlist_id = p.id
		LEFT JOIN tracks t ON t.id = i.track_id
		GROUP BY p.id
		ORDER BY p.created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []Playlist{}
	for rows.Next() {
		var p Playlist
		rows.Scan(&p.ID, &p.Name, &p.TrackCount, &p.TotalDuration, &p.CreatedAt)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

type createReq struct {
	Name string `json:"name"`
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", 400)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO playlists (name) VALUES (?)`, req.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, Playlist{ID: id, Name: req.Name})
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var p PlaylistWithTracks
	err := a.db.QueryRowContext(r.Context(), `
		SELECT id, name, created_at FROM playlists WHERE id=?`, id).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT `+models.TrackCols+`
		FROM playlist_items i
		JOIN tracks ON tracks.id = i.track_id
		WHERE i.playlist_id = ?
		ORDER BY i.position`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		t, _ := models.ScanTrack(rows)
		p.Tracks = append(p.Tracks, t)
		p.TotalDuration += int(t.DurationSec)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	p.TrackCount = len(p.Tracks)
	writeJSON(w, p)
}

type updateReq struct {
	Name string `json:"name"`
}

func (a *API) update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", 400)
		return
	}
	_, err := a.db.ExecContext(r.Context(), `UPDATE playlists SET name=? WHERE id=?`, req.Name, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM playlists WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

type addTrackReq struct {
	TrackID int64 `json:"track_id"`
}

func (a *API) addTrack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req addTrackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.TrackID <= 0 {
		http.Error(w, "track_id is required", 400)
		return
	}
	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO playlist_items (playlist_id, track_id, position)
		SELECT ?, ?, COALESCE((SELECT MAX(position)+1 FROM playlist_items WHERE playlist_id=?), 0)`,
		id, req.TrackID, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) removeTrack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	position, _ := strconv.ParseInt(chi.URLParam(r, "position"), 10, 64)
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM playlist_items WHERE playlist_id=? AND position=?`, id, position)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
