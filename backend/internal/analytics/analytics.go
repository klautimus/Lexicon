package analytics

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type API struct{ db *sql.DB }

func New(db *sql.DB) *API { return &API{db: db} }

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
	a.db.QueryRow(`SELECT COUNT(*) FROM plays`).Scan(&o.TotalPlays)
	a.db.QueryRow(`SELECT COUNT(DISTINCT track_id) FROM plays`).Scan(&o.UniqueTracks)
	a.db.QueryRow(`SELECT IFNULL(SUM(duration_played_sec),0) FROM plays`).Scan(&o.ListenSec)
	if o.TotalPlays > 0 {
		var c int
		a.db.QueryRow(`SELECT SUM(completed) FROM plays`).Scan(&c)
		o.CompletedPct = c * 100 / o.TotalPlays
	}
	writeJSON(w, o)
}

func (a *API) topArtists(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.db.Query(`
		SELECT IFNULL(COALESCE(NULLIF(t.album_artist,''),t.artist),'') AS artist, COUNT(*), IFNULL(SUM(p.duration_played_sec),0)
		FROM plays p JOIN tracks t ON t.id=p.track_id
		GROUP BY artist HAVING artist!='' ORDER BY COUNT(*) DESC LIMIT 20`)
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
	writeJSON(w, out)
}

func (a *API) topTracks(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.db.Query(`
		SELECT t.id, t.title, IFNULL(t.artist,''), COUNT(*) FROM plays p JOIN tracks t ON t.id=p.track_id
		GROUP BY t.id ORDER BY COUNT(*) DESC LIMIT 20`)
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
	writeJSON(w, out)
}

func (a *API) topGenres(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.db.Query(`
		SELECT IFNULL(t.genre,''), COUNT(*) FROM plays p JOIN tracks t ON t.id=p.track_id
		GROUP BY t.genre HAVING t.genre!='' ORDER BY COUNT(*) DESC LIMIT 15`)
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
	writeJSON(w, out)
}

func (a *API) heatmap(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.db.Query(`
		SELECT CAST(strftime('%w', started_at, 'unixepoch','localtime') AS INTEGER) AS dow,
		       CAST(strftime('%H', started_at, 'unixepoch','localtime') AS INTEGER) AS hour,
		       COUNT(*) FROM plays GROUP BY dow, hour`)
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
	writeJSON(w, out)
}
