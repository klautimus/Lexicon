package library

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type API struct{ db *sql.DB }

func New(db *sql.DB) *API { return &API{db: db} }

type Track struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	AlbumArtist string `json:"album_artist"`
	Album       string `json:"album"`
	TrackNo     int    `json:"track_no"`
	DiscNo      int    `json:"disc_no"`
	Year        int    `json:"year"`
	Genre       string `json:"genre"`
	DurationSec int    `json:"duration_sec"`
	MediaKind   string `json:"media_kind"`
	Mime        string `json:"mime"`
	SpotifyID   string `json:"spotify_id,omitempty"`
	ExternalURL string `json:"external_url,omitempty"`
}

const trackCols = `id,title,IFNULL(artist,''),IFNULL(album_artist,''),IFNULL(album,''),IFNULL(track_no,0),IFNULL(disc_no,0),IFNULL(year,0),IFNULL(genre,''),IFNULL(duration_sec,0),media_kind,IFNULL(mime,''),IFNULL(spotify_id,''),IFNULL(external_url,'')`

func scanTrack(rows interface {
	Scan(...interface{}) error
}) (Track, error) {
	var t Track
	err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.AlbumArtist, &t.Album, &t.TrackNo, &t.DiscNo, &t.Year, &t.Genre, &t.DurationSec, &t.MediaKind, &t.Mime, &t.SpotifyID, &t.ExternalURL)
	return t, err
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/library/tracks", a.tracks)
	r.Get("/api/library/albums", a.albums)
	r.Get("/api/library/artists", a.artists)
	r.Get("/api/library/podcasts", a.podcasts)
	r.Get("/api/library/search", a.search)
	r.Get("/api/library/track/{id}", a.track)
	r.Delete("/api/library/track/{id}", a.deleteTrack)
	r.Get("/api/library/cover/{id}", a.cover)
	r.Get("/api/library/stats", a.stats)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (a *API) tracks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	kind := r.URL.Query().Get("kind")

	var total int
	countQ := `SELECT COUNT(*) FROM tracks`
	countArgs := []interface{}{}
	if kind != "" {
		countQ += ` WHERE media_kind=?`
		countArgs = append(countArgs, kind)
	}
	a.db.QueryRowContext(r.Context(), countQ, countArgs...).Scan(&total)

	q := `SELECT ` + trackCols + ` FROM tracks`
	args := []interface{}{}
	if kind != "" {
		q += ` WHERE media_kind=?`
		args = append(args, kind)
	}
	q += ` ORDER BY artist, album, disc_no, track_no, title LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := a.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []Track{}
	for rows.Next() {
		t, _ := scanTrack(rows)
		out = append(out, t)
	}
	writeJSON(w, map[string]interface{}{"tracks": out, "total": total})
}

func (a *API) albums(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(album,'') AS album, IFNULL(COALESCE(NULLIF(album_artist,''),artist),'') AS artist, IFNULL(year,0), COUNT(*)
		FROM tracks WHERE media_kind='music' AND IFNULL(album,'')!=''
		GROUP BY album, artist ORDER BY artist, album`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Album struct {
		Album    string `json:"album"`
		Artist   string `json:"artist"`
		Year     int    `json:"year"`
		Tracks   int    `json:"tracks"`
	}
	out := []Album{}
	for rows.Next() {
		var x Album
		rows.Scan(&x.Album, &x.Artist, &x.Year, &x.Tracks)
		out = append(out, x)
	}
	writeJSON(w, out)
}

func (a *API) artists(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(COALESCE(NULLIF(album_artist,''),artist),'') AS artist, COUNT(*) AS tracks, COUNT(DISTINCT album) AS albums
		FROM tracks WHERE media_kind='music' GROUP BY artist HAVING artist!='' ORDER BY artist`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Artist struct {
		Artist string `json:"artist"`
		Tracks int    `json:"tracks"`
		Albums int    `json:"albums"`
	}
	out := []Artist{}
	for rows.Next() {
		var x Artist
		rows.Scan(&x.Artist, &x.Tracks, &x.Albums)
		out = append(out, x)
	}
	writeJSON(w, out)
}

func (a *API) podcasts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(COALESCE(NULLIF(album,''),album_artist,artist),'') AS show, COUNT(*) AS episodes
		FROM tracks WHERE media_kind='podcast' GROUP BY show HAVING show!='' ORDER BY show`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Show struct {
		Show     string `json:"show"`
		Episodes int    `json:"episodes"`
	}
	out := []Show{}
	for rows.Next() {
		var x Show
		rows.Scan(&x.Show, &x.Episodes)
		out = append(out, x)
	}
	writeJSON(w, out)
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, []Track{})
		return
	}
	// FTS5: quote terms with AND
	parts := strings.Fields(q)
	for i, p := range parts {
		parts[i] = `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
	}
	ftsQ := strings.Join(parts, " AND ")
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT t.id,t.title,IFNULL(t.artist,''),IFNULL(t.album_artist,''),IFNULL(t.album,''),IFNULL(t.track_no,0),IFNULL(t.disc_no,0),IFNULL(t.year,0),IFNULL(t.genre,''),IFNULL(t.duration_sec,0),t.media_kind,IFNULL(t.mime,''),IFNULL(t.spotify_id,''),IFNULL(t.external_url,'')
		FROM tracks_fts f JOIN tracks t ON t.id=f.rowid
		WHERE tracks_fts MATCH ? ORDER BY rank LIMIT 100`, ftsQ)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []Track{}
	for rows.Next() {
		t, _ := scanTrack(rows)
		out = append(out, t)
	}
	writeJSON(w, out)
}

func (a *API) track(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := a.db.QueryRowContext(r.Context(), `SELECT `+trackCols+` FROM tracks WHERE id=?`, id)
	t, err := scanTrack(row)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	writeJSON(w, t)
}

func (a *API) deleteTrack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	var path string
	if err := a.db.QueryRowContext(r.Context(), `SELECT path FROM tracks WHERE id=?`, id).Scan(&path); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if path != "" {
		os.Remove(path)
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM tracks WHERE id=?`, id); err != nil {
		http.Error(w, "delete failed", 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) cover(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var path string
	if err := a.db.QueryRowContext(r.Context(), `SELECT path FROM tracks WHERE id=?`, id).Scan(&path); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	// Read embedded art via dhowden/tag
	f, err := openReader(path)
	if err != nil {
		http.Error(w, "", 404)
		return
	}
	defer f.Close()
	pic := readCover(f)
	if pic == nil {
		http.Error(w, "no cover", 404)
		return
	}
	w.Header().Set("Content-Type", pic.MIMEType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(pic.Data)
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	type Stats struct {
		Tracks   int `json:"tracks"`
		Albums   int `json:"albums"`
		Artists  int `json:"artists"`
		Podcasts int `json:"podcasts"`
	}
	var s Stats
	a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tracks WHERE media_kind='music'`).Scan(&s.Tracks)
	a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT album) FROM tracks WHERE media_kind='music' AND IFNULL(album,'')!=''`).Scan(&s.Albums)
	a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT COALESCE(NULLIF(album_artist,''),artist)) FROM tracks WHERE media_kind='music'`).Scan(&s.Artists)
	a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT COALESCE(NULLIF(album,''),album_artist,artist)) FROM tracks WHERE media_kind='podcast'`).Scan(&s.Podcasts)
	writeJSON(w, s)
}
