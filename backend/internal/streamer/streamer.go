package streamer

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// allowedMIMETypes is the whitelist of permitted Content-Type values for streaming.
// BUG-BUILD-13: MIME types from the DB must be validated to prevent content-type confusion.
var allowedMIMETypes = map[string]bool{
	"audio/mpeg":       true,
	"audio/mp3":        true,
	"audio/flac":       true,
	"audio/ogg":        true,
	"audio/opus":       true,
	"audio/aac":        true,
	"audio/mp4":        true,
	"audio/x-m4a":      true,
	"audio/wav":        true,
	"audio/x-wav":      true,
	"audio/webm":       true,
	"audio/x-ms-wma":   true,
	"audio/aiff":       true,
	"audio/x-aiff":     true,
	"image/jpeg":       true,
	"image/png":        true,
	"image/gif":        true,
	"image/webp":       true,
	"image/bmp":        true,
	"application/octet-stream": true,
}

func isValidMIME(mime string) bool {
	return allowedMIMETypes[strings.ToLower(strings.TrimSpace(mime))]
}

type Streamer struct {
	db     *sql.DB
	roots  []string
}

func New(db *sql.DB, mediaRoots string) *Streamer {
	var roots []string
	for _, r := range strings.Split(mediaRoots, ";") {
		if r = strings.TrimSpace(r); r != "" {
			roots = append(roots, filepath.Clean(r))
		}
	}
	return &Streamer{db: db, roots: roots}
}

func (s *Streamer) Mount(r chi.Router) {
	r.Get("/api/stream/{id}", s.stream)
}

func (s *Streamer) stream(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range, Accept-Encoding")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
		w.WriteHeader(http.StatusOK)
		return
	}
	// BUG-BUILD-14: Return 400 for invalid track IDs
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		log.Printf("[stream] invalid track ID: %q", idStr)
		http.Error(w, `{"error":"invalid track ID"}`, http.StatusBadRequest)
		return
	}
	var (
		path string
		mime string
	)
	if err := s.db.QueryRowContext(r.Context(), `SELECT path, IFNULL(mime,'application/octet-stream') FROM tracks WHERE id=?`, id).Scan(&path, &mime); err != nil {
		log.Printf("[stream] stream track %d: lookup: %v", id, err)
		http.Error(w, "not found", 404)
		return
	}

	// BUG-BUILD-13: Validate MIME type against whitelist
	if !isValidMIME(mime) {
		log.Printf("[stream] track %d: disallowed MIME type %q, falling back to application/octet-stream", id, mime)
		mime = "application/octet-stream"
	}

	// Path traversal guard: resolve symlinks and verify path is within allowed roots
	if len(s.roots) > 0 {
		resolved, err := ValidatePath(path, s.roots)
		if err != nil {
			log.Printf("[stream] stream track %d: %v", id, err)
			http.Error(w, "forbidden", 403)
			return
		}
		// Use the resolved (symlink-expanded) path for the actual file open
		path = resolved
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[stream] stream track %d: file not found: %s", id, path)
			http.Error(w, "file not found", 404)
		} else {
			log.Printf("[stream] stream track %d: open %s: %v", id, path)
			http.Error(w, "open", 500)
		}
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		log.Printf("[stream] stream track %d: stat %s: %v", id, path)
		http.Error(w, "stat", 500)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Accept-Encoding")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}
