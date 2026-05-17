package analytics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type API struct{ db *sql.DB; timezone string }

func New(db *sql.DB, timezone string) *API { return &API{db: db, timezone: timezone} }

func (a *API) Mount(r chi.Router) {
	r.Get("/api/analytics/overview", a.overview)
	r.Get("/api/analytics/top-artists", a.topArtists)
	r.Get("/api/analytics/top-tracks", a.topTracks)
	r.Get("/api/analytics/top-genres", a.topGenres)
	r.Get("/api/analytics/heatmap", a.heatmap)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	type O struct {
		TotalPlays   int `json:"total_plays"`
		UniqueTracks int `json:"unique_tracks"`
		ListenSec    int `json:"listen_sec"`
		CompletedPct int `json:"completed_pct"`
	}
	var o O
	a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM plays`).Scan(&o.TotalPlays)
	a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT track_id) FROM plays`).Scan(&o.UniqueTracks)
	a.db.QueryRowContext(r.Context(), `SELECT IFNULL(SUM(duration_played_sec),0) FROM plays`).Scan(&o.ListenSec)
	if o.TotalPlays > 0 {
		var c int
		a.db.QueryRowContext(r.Context(), `SELECT SUM(completed) FROM plays`).Scan(&c)
		o.CompletedPct = c * 100 / o.TotalPlays
	}
	writeJSON(w, o)
}

func (a *API) topArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(COALESCE(NULLIF(t.album_artist,''),t.artist),'') AS artist, COUNT(*), IFNULL(SUM(p.duration_played_sec),0)
		FROM plays p JOIN tracks t ON t.id=p.track_id
		GROUP BY artist HAVING artist!='' ORDER BY COUNT(*) DESC LIMIT 20`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Row struct {
		Artist    string `json:"artist"`
		Plays     int    `json:"plays"`
		ListenSec int    `json:"listen_sec"`
	}
	out := []Row{}
	for rows.Next() {
		var x Row
		rows.Scan(&x.Artist, &x.Plays, &x.ListenSec)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) topTracks(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT t.id, t.title, IFNULL(t.artist,''), COUNT(*) FROM plays p JOIN tracks t ON t.id=p.track_id
		GROUP BY t.id ORDER BY COUNT(*) DESC LIMIT 20`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Row struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Artist string `json:"artist"`
		Plays  int    `json:"plays"`
	}
	out := []Row{}
	for rows.Next() {
		var x Row
		rows.Scan(&x.ID, &x.Title, &x.Artist, &x.Plays)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) topGenres(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(t.genre,''), COUNT(*) FROM plays p JOIN tracks t ON t.id=p.track_id
		GROUP BY t.genre HAVING t.genre!='' ORDER BY COUNT(*) DESC LIMIT 15`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Row struct {
		Genre string `json:"genre"`
		Plays int    `json:"plays"`
	}
	out := []Row{}
	for rows.Next() {
		var x Row
		rows.Scan(&x.Genre, &x.Plays)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) heatmap(w http.ResponseWriter, r *http.Request) {
	tzMod := a.timezone
	if tzMod == "local" {
		tzMod = "localtime"
	}
	q := fmt.Sprintf(`SELECT CAST(strftime('%%w', started_at, 'unixepoch', '%s') AS INTEGER) AS dow,
	       CAST(strftime('%%H', started_at, 'unixepoch', '%s') AS INTEGER) AS hour,
	       COUNT(*) FROM plays GROUP BY dow, hour`, tzMod, tzMod)
	rows, err := a.db.QueryContext(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Cell struct {
		Dow   int `json:"dow"`
		Hour  int `json:"hour"`
		Plays int `json:"plays"`
	}
	out := []Cell{}
	for rows.Next() {
		var c Cell
		rows.Scan(&c.Dow, &c.Hour, &c.Plays)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}
