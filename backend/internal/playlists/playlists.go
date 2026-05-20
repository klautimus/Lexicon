package playlists

import (
	"database/sql"
	"encoding/json"
	"log"
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

type PlaylistTrack struct {
	models.Track
	Position int `json:"position"`
}

type PlaylistWithTracks struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	TrackCount    int             `json:"track_count"`
	TotalDuration int             `json:"total_duration"`
	CreatedAt     int64           `json:"created_at"`
	Tracks        []PlaylistTrack `json:"tracks"`
}

const maxBodySize = 1 << 20 // 1 MB

type API struct{ db *sql.DB }

func New(db *sql.DB) *API { return &API{db: db} }

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[playlists] writeJSON encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("[playlists] writeError encode: %v", err)
	}
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
		log.Printf("[playlists] list query: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []Playlist{}
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(&p.ID, &p.Name, &p.TrackCount, &p.TotalDuration, &p.CreatedAt); err != nil {
			log.Printf("[playlists] list scan: %v", err)
			writeError(w, err.Error(), 500)
			return
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[playlists] list rows: %v", err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

type createReq struct {
	Name string `json:"name"`
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[playlists] create decode: %v", err)
		writeError(w, err.Error(), 400)
		return
	}
	if req.Name == "" {
		writeError(w, "name is required", 400)
		return
	}
	if len(req.Name) > 256 {
		writeError(w, "name must be 256 characters or fewer", 400)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO playlists (name) VALUES (?)`, req.Name)
	if err != nil {
		log.Printf("[playlists] create insert: %v", err)
		writeError(w, err.Error(), 500)
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
		log.Printf("[playlists] get playlist %d: %v", id, err)
		writeError(w, "not found", 404)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT `+models.TrackColsAliased("tracks")+`, i.position
		FROM playlist_items i
		JOIN tracks ON tracks.id = i.track_id
		WHERE i.playlist_id = ?
		ORDER BY i.position`, id)
	if err != nil {
		log.Printf("[playlists] get tracks query playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t models.Track
		var path, title, artist, albumArtist, album, genre, mediaKind, mime sql.NullString
		var spotifyID, externalURL sql.NullString
		var trackNo, discNo, year, durationSec sql.NullInt64
		var sizeBytes, addedAt, mtime sql.NullInt64
		var coverPath sql.NullString
		var loudnessIntegrated, loudnessTruePeak, loudnessRange sql.NullFloat64
		var position int
		err := rows.Scan(
			&t.ID,
			&path, &title, &artist, &albumArtist, &album,
			&trackNo, &discNo, &year, &genre,
			&durationSec, &mediaKind, &mime,
			&sizeBytes, &coverPath, &addedAt, &mtime,
			&loudnessIntegrated, &loudnessTruePeak, &loudnessRange,
			&spotifyID, &externalURL,
			&position,
		)
		if err != nil {
			log.Printf("[playlists] get track scan playlist %d: %v", id, err)
			writeError(w, err.Error(), 500)
			return
		}
		if path.Valid { t.Path = path.String }
		if title.Valid { t.Title = title.String }
		if artist.Valid { t.Artist = artist.String }
		if albumArtist.Valid { t.AlbumArtist = albumArtist.String }
		if album.Valid { t.Album = album.String }
		if trackNo.Valid { t.TrackNo = int(trackNo.Int64) }
		if discNo.Valid { t.DiscNo = int(discNo.Int64) }
		if year.Valid { t.Year = int(year.Int64) }
		if genre.Valid { t.Genre = genre.String }
		if durationSec.Valid { t.DurationSec = int(durationSec.Int64) }
		if mediaKind.Valid { t.MediaKind = mediaKind.String }
		if mime.Valid { t.Mime = mime.String }
		if sizeBytes.Valid { t.SizeBytes = sizeBytes.Int64 }
		if coverPath.Valid { t.CoverPath = coverPath.String }
		if addedAt.Valid { t.AddedAt = addedAt.Int64 }
		if mtime.Valid { t.Mtime = mtime.Int64 }
		if loudnessIntegrated.Valid { t.LoudnessIntegrated = loudnessIntegrated.Float64 }
		if loudnessTruePeak.Valid { t.LoudnessTruePeak = loudnessTruePeak.Float64 }
		if loudnessRange.Valid { t.LoudnessRange = loudnessRange.Float64 }
		if spotifyID.Valid { t.SpotifyID = spotifyID.String }
		if externalURL.Valid { t.ExternalURL = externalURL.String }
		p.Tracks = append(p.Tracks, PlaylistTrack{Track: t, Position: position})
		p.TotalDuration += int(t.DurationSec)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[playlists] get rows err playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	p.TrackCount = len(p.Tracks)
	if p.Tracks == nil {
		p.Tracks = []PlaylistTrack{}
	}
	writeJSON(w, p)
}

type updateReq struct {
	Name string `json:"name"`
}

func (a *API) update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req updateReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[playlists] update decode playlist %d: %v", id, err)
		writeError(w, err.Error(), 400)
		return
	}
	if req.Name == "" {
		writeError(w, "name is required", 400)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE playlists SET name=? WHERE id=?`, req.Name, id)
	if err != nil {
		log.Printf("[playlists] update exec playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("[playlists] update rowsAffected playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	if rows == 0 {
		writeError(w, "playlist not found", 404)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM playlists WHERE id=?`, id)
	if err != nil {
		log.Printf("[playlists] delete exec playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("[playlists] delete rowsAffected playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	if rows == 0 {
		writeError(w, "playlist not found", 404)
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[playlists] addTrack decode playlist %d: %v", id, err)
		writeError(w, err.Error(), 400)
		return
	}
	if req.TrackID <= 0 {
		writeError(w, "track_id is required", 400)
		return
	}
	// Validate track exists before adding to playlist
	var exists int
	if err := a.db.QueryRowContext(r.Context(), `SELECT 1 FROM tracks WHERE id=?`, req.TrackID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, "track not found", 404)
			return
		}
		log.Printf("[playlists] addTrack track lookup playlist %d track %d: %v", id, req.TrackID, err)
		writeError(w, err.Error(), 500)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		log.Printf("[playlists] addTrack begin tx playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	var maxPos sql.NullInt64
	if err := tx.QueryRowContext(r.Context(),
		`SELECT MAX(position) FROM playlist_items WHERE playlist_id=?`,
		id).Scan(&maxPos); err != nil {
		log.Printf("[playlists] addTrack max position playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	nextPos := int64(0)
	if maxPos.Valid {
		nextPos = maxPos.Int64 + 1
	}

	res, err := tx.ExecContext(r.Context(),
		`INSERT INTO playlist_items (playlist_id, track_id, position) VALUES (?, ?, ?)`,
		id, req.TrackID, nextPos)
	if err != nil {
		log.Printf("[playlists] addTrack insert playlist %d track %d: %v", id, req.TrackID, err)
		writeError(w, err.Error(), 500)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("[playlists] addTrack rowsAffected playlist %d track %d: %v", id, req.TrackID, err)
		writeError(w, err.Error(), 500)
		return
	}
	if rows == 0 {
		log.Printf("[playlists] addTrack no rows affected playlist %d track %d", id, req.TrackID)
		writeError(w, "track not added", 500)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("[playlists] addTrack commit playlist %d: %v", id, err)
		writeError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) removeTrack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	position, _ := strconv.ParseInt(chi.URLParam(r, "position"), 10, 64)
	if position < 0 {
		writeError(w, "position must be >= 0", 400)
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM playlist_items WHERE playlist_id=? AND position=?`, id, position)
	if err != nil {
		log.Printf("[playlists] removeTrack exec playlist %d position %d: %v", id, position, err)
		writeError(w, err.Error(), 500)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("[playlists] removeTrack rowsAffected playlist %d position %d: %v", id, position, err)
		writeError(w, err.Error(), 500)
		return
	}
	if rows == 0 {
		writeError(w, "track not found", 404)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
