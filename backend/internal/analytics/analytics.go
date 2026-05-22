package analytics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/auth"
)

// validSQLiteTimeModifiers is a whitelist of safe SQLite strftime modifiers
// for timezone conversion. Never pass user input directly to SQL.
var validSQLiteTimeModifiers = map[string]bool{
	"localtime":        true,
	"utc":              true,
	"+00:00":           true,
	"+01:00":           true,
	"+02:00":           true,
	"+03:00":           true,
	"+04:00":           true,
	"+05:00":           true,
	"+06:00":           true,
	"+07:00":           true,
	"+08:00":           true,
	"+09:00":           true,
	"+10:00":           true,
	"+11:00":           true,
	"+12:00":           true,
	"-01:00":           true,
	"-02:00":           true,
	"-03:00":           true,
	"-04:00":           true,
	"-05:00":           true,
	"-06:00":           true,
	"-07:00":           true,
	"-08:00":           true,
	"-09:00":           true,
	"-10:00":           true,
	"-11:00":           true,
	"-12:00":           true,
}

// normalizeTimezone validates and returns a safe SQLite timezone modifier.
// Falls back to "localtime" if the value is not in the whitelist.
func normalizeTimezone(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "local" {
		tz = "localtime"
	}
	if validSQLiteTimeModifiers[tz] {
		return tz
	}
	return "localtime"
}

type API struct{ db *sql.DB; timezone string }

func getUserID(r *http.Request) int64 {
	if u, ok := auth.UserFromContext(r.Context()); ok {
		return u.UserID
	}
	return 0
}

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
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[analytics] writeJSON encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	type O struct {
		TotalPlays   int `json:"total_plays"`
		UniqueTracks int `json:"unique_tracks"`
		ListenSec    int `json:"listen_sec"`
		CompletedPct int `json:"completed_pct"`
	}
	var o O
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM plays WHERE user_id IS NULL OR user_id = ?`, getUserID(r)).Scan(&o.TotalPlays); err != nil {
		log.Printf("[analytics] overview total_plays: %v", err)
		writeError(w, "failed to load analytics", 500)
		return
	}
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT track_id) FROM plays WHERE user_id IS NULL OR user_id = ?`, getUserID(r)).Scan(&o.UniqueTracks); err != nil {
		log.Printf("[analytics] overview unique_tracks: %v", err)
		writeError(w, "failed to load analytics", 500)
		return
	}
	if err := a.db.QueryRowContext(r.Context(), `SELECT IFNULL(SUM(duration_played_sec),0) FROM plays WHERE user_id IS NULL OR user_id = ?`, getUserID(r)).Scan(&o.ListenSec); err != nil {
		log.Printf("[analytics] overview listen_sec: %v", err)
		writeError(w, "failed to load analytics", 500)
		return
	}
	if o.TotalPlays > 0 {
		var c int
		if err := a.db.QueryRowContext(r.Context(), `SELECT SUM(completed) FROM plays WHERE user_id IS NULL OR user_id = ?`, getUserID(r)).Scan(&c); err != nil {
			log.Printf("[analytics] overview completed: %v", err)
			writeError(w, "failed to load analytics", 500)
			return
		}
		o.CompletedPct = int(math.Round(float64(c) * 100.0 / float64(o.TotalPlays)))
	}
	writeJSON(w, o)
}

func (a *API) topArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(COALESCE(NULLIF(t.album_artist,''),t.artist),'') AS artist, COUNT(*), IFNULL(SUM(p.duration_played_sec),0)
		FROM plays p LEFT JOIN tracks t ON t.id=p.track_id WHERE p.user_id IS NULL OR p.user_id=?
		GROUP BY artist HAVING artist!='' ORDER BY COUNT(*) DESC LIMIT 20`, getUserID(r))
	if err != nil {
		log.Printf("[analytics] topArtists query: %v", err)
		writeError(w, "failed to load data", 500)
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
		if err := rows.Scan(&x.Artist, &x.Plays, &x.ListenSec); err != nil {
			log.Printf("[analytics] topArtists scan: %v", err)
			writeError(w, "failed to load data", 500)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[analytics] topArtists rows: %v", err)
		writeError(w, "failed to load data", 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) topTracks(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT t.id, IFNULL(t.title,'(deleted)'), IFNULL(t.artist,''), COUNT(*) FROM plays p LEFT JOIN tracks t ON t.id=p.track_id
		WHERE p.user_id IS NULL OR p.user_id=?
		GROUP BY t.id ORDER BY COUNT(*) DESC LIMIT 20`, getUserID(r))
	if err != nil {
		log.Printf("[analytics] topTracks query: %v", err)
		writeError(w, "failed to load data", 500)
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
		if err := rows.Scan(&x.ID, &x.Title, &x.Artist, &x.Plays); err != nil {
			log.Printf("[analytics] topTracks scan: %v", err)
			writeError(w, "failed to load data", 500)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[analytics] topTracks rows: %v", err)
		writeError(w, "failed to load data", 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) topGenres(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(t.genre,''), COUNT(*) FROM plays p LEFT JOIN tracks t ON t.id=p.track_id
		WHERE p.user_id IS NULL OR p.user_id=?
		GROUP BY t.genre HAVING t.genre!='' ORDER BY COUNT(*) DESC LIMIT 15`, getUserID(r))
	if err != nil {
		log.Printf("[analytics] topGenres query: %v", err)
		writeError(w, "failed to load data", 500)
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
		if err := rows.Scan(&x.Genre, &x.Plays); err != nil {
			log.Printf("[analytics] topGenres scan: %v", err)
			writeError(w, "failed to load data", 500)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[analytics] topGenres rows: %v", err)
		writeError(w, "failed to load data", 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) heatmap(w http.ResponseWriter, r *http.Request) {
	tzMod := normalizeTimezone(a.timezone)
	q := fmt.Sprintf(`SELECT CAST(strftime('%%w', started_at, 'unixepoch', '%s') AS INTEGER) AS dow,
	       CAST(strftime('%%H', started_at, 'unixepoch', '%s') AS INTEGER) AS hour,
	       COUNT(*) FROM plays WHERE user_id IS NULL OR user_id = ? GROUP BY dow, hour`, tzMod, tzMod)
	rows, err := a.db.QueryContext(r.Context(), q, getUserID(r))
	if err != nil {
		log.Printf("[analytics] heatmap query: %v", err)
		writeError(w, "failed to load data", 500)
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
		if err := rows.Scan(&c.Dow, &c.Hour, &c.Plays); err != nil {
			log.Printf("[analytics] heatmap scan: %v", err)
			writeError(w, "failed to load data", 500)
			return
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[analytics] heatmap rows: %v", err)
		writeError(w, "failed to load data", 500)
		return
	}
	writeJSON(w, out)
}
