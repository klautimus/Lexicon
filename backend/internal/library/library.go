package library

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/models"
)

type API struct {
	db         *sql.DB
	mediaRoots []string
}

func New(db *sql.DB, mediaRoots []string) *API {
	cleaned := make([]string, 0, len(mediaRoots))
	for _, r := range mediaRoots {
		r = strings.TrimSpace(r)
		if r != "" {
			cleaned = append(cleaned, filepath.Clean(r))
		}
	}
	return &API{db: db, mediaRoots: cleaned}
}

// isPathSafe returns true if the given path is within one of the configured media roots.
func (a *API) isPathSafe(path string) bool {
	if len(a.mediaRoots) == 0 {
		return true // no roots configured, allow (scanner hasn't run yet)
	}
	clean := filepath.Clean(path)
	sep := string(os.PathSeparator)
	for _, root := range a.mediaRoots {
		if strings.HasPrefix(clean, root+sep) || clean == root {
			return true
		}
	}
	return false
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
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[library] writeJSON encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("[library] writeError encode: %v", err)
	}
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
	if err := a.db.QueryRowContext(r.Context(), countQ, countArgs...).Scan(&total); err != nil {
		log.Printf("[library] tracks count: %v", err)
		writeError(w, err.Error(), 500)
		return
	}

	q := `SELECT ` + models.TrackCols + ` FROM tracks`
	args := []interface{}{}
	if kind != "" {
		q += ` WHERE media_kind=?`
		args = append(args, kind)
	}
	q += ` ORDER BY artist, album, disc_no, track_no, title LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := a.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		log.Printf("[library] tracks query: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []models.Track{}
	for rows.Next() {
		t, err := models.ScanTrack(rows)
		if err != nil {
			log.Printf("[library] tracks scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[library] tracks rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"tracks": out, "total": total})
}

func (a *API) albums(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(album,'') AS album, IFNULL(COALESCE(NULLIF(album_artist,''),artist),'') AS artist, IFNULL(year,0), COUNT(*)
		FROM tracks WHERE media_kind='music' AND IFNULL(album,'')!=''
		GROUP BY album, artist ORDER BY artist, album`)
	if err != nil {
		log.Printf("[library] albums query: %v", err)
		writeError(w, err.Error(), 500)
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
		if err := rows.Scan(&x.Album, &x.Artist, &x.Year, &x.Tracks); err != nil {
			log.Printf("[library] albums scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[library] albums rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) artists(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(COALESCE(NULLIF(album_artist,''),artist),'') AS artist, COUNT(*) AS tracks, COUNT(DISTINCT album) AS albums
		FROM tracks WHERE media_kind='music' GROUP BY artist HAVING artist!='' ORDER BY artist`)
	if err != nil {
		log.Printf("[library] artists query: %v", err)
		writeError(w, err.Error(), 500)
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
		if err := rows.Scan(&x.Artist, &x.Tracks, &x.Albums); err != nil {
			log.Printf("[library] artists scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[library] artists rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) podcasts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT IFNULL(COALESCE(NULLIF(album,''),album_artist,artist),'') AS show, COUNT(*) AS episodes
		FROM tracks WHERE media_kind='podcast' GROUP BY show HAVING show!='' ORDER BY show`)
	if err != nil {
		log.Printf("[library] podcasts query: %v", err)
		writeError(w, err.Error(), 500)
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
		if err := rows.Scan(&x.Show, &x.Episodes); err != nil {
			log.Printf("[library] podcasts scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[library] podcasts rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, []models.Track{})
		return
	}
	if len(q) > 256 {
		writeError(w, "query must be 256 characters or fewer", 400)
		return
	}
	// FTS5: quote terms with AND
	parts := strings.Fields(q)
	for i, p := range parts {
		parts[i] = `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
	}
	ftsQ := strings.Join(parts, " AND ")
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT `+models.TrackColsAliased("t")+`
		FROM tracks_fts f JOIN tracks t ON t.id=f.rowid
		WHERE tracks_fts MATCH ? ORDER BY rank LIMIT 100`, ftsQ)
	if err != nil {
		log.Printf("[library] search fts query: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []models.Track{}
	for rows.Next() {
		t, err := models.ScanTrack(rows)
		if err != nil {
			log.Printf("[library] search scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[library] search rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func (a *API) track(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := a.db.QueryRowContext(r.Context(), `SELECT `+models.TrackCols+` FROM tracks WHERE id=?`, id)
	t, err := models.ScanTrack(row)
	if err != nil {
		log.Printf("[library] track get id %d: %v", id, err)
		writeError(w, "not found", 404)
		return
	}
	writeJSON(w, t)
}

func (a *API) deleteTrack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		writeError(w, "invalid id", 400)
		return
	}
	var path string
	if err := a.db.QueryRowContext(r.Context(), `SELECT path FROM tracks WHERE id=?`, id).Scan(&path); err != nil {
		log.Printf("[library] deleteTrack lookup id %d: %v", id, err)
		writeError(w, "not found", 404)
		return
	}
	if !a.isPathSafe(path) {
		log.Printf("[library] deleteTrack path unsafe id %d path %s", id, path)
		writeError(w, "forbidden", 403)
		return
	}
	// Wrap both DB delete and file removal in a transaction so they
	// succeed or fail together. If the file delete fails the DB change
	// is rolled back and the operation can be retried.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		log.Printf("[library] deleteTrack beginTx id %d: %v", id, err)
		writeError(w, "delete failed", 500)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM tracks WHERE id=?`, id); err != nil {
		tx.Rollback()
		log.Printf("[library] deleteTrack exec id %d: %v", id, err)
		writeError(w, "delete failed", 500)
		return
	}
	if path != "" {
		if err := os.Remove(path); err != nil {
			tx.Rollback()
			log.Printf("[library] deleteTrack remove id %d path %s: %v", id, path, err)
			writeError(w, "delete failed", 500)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[library] deleteTrack commit id %d: %v", id, err)
		writeError(w, "delete failed", 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) cover(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var path string
	if err := a.db.QueryRowContext(r.Context(), `SELECT path FROM tracks WHERE id=?`, id).Scan(&path); err != nil {
		log.Printf("[library] cover lookup id %d: %v", id, err)
		writeError(w, "not found", 404)
		return
	}
	// Read embedded art via dhowden/tag
	if !a.isPathSafe(path) {
		log.Printf("[library] cover path unsafe id %d path %s", id, path)
		writeError(w, "forbidden", 403)
		return
	}
	f, err := openReader(path)
	if err != nil {
		log.Printf("[library] cover open id %d path %s: %v", id, path, err)
		writeError(w, "", 404)
		return
	}
	defer f.Close()
	pic := readCover(f)
	if pic == nil {
		writeError(w, "no cover", 404)
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
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tracks WHERE media_kind='music'`).Scan(&s.Tracks); err != nil {
		log.Printf("[library] stats tracks: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT album) FROM tracks WHERE media_kind='music' AND IFNULL(album,'')!=''`).Scan(&s.Albums); err != nil {
		log.Printf("[library] stats albums: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT COALESCE(NULLIF(album_artist,''),artist)) FROM tracks WHERE media_kind='music'`).Scan(&s.Artists); err != nil {
		log.Printf("[library] stats artists: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT COALESCE(NULLIF(album,''),album_artist,artist)) FROM tracks WHERE media_kind='podcast'`).Scan(&s.Podcasts); err != nil {
		log.Printf("[library] stats podcasts: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, s)
}
