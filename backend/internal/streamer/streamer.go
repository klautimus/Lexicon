package streamer

import (
	"database/sql"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Streamer struct{ db *sql.DB }

func New(db *sql.DB) *Streamer { return &Streamer{db: db} }

func (s *Streamer) Mount(r chi.Router) {
	r.Get("/api/stream/{id}", s.stream)
}

func (s *Streamer) stream(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var (
		path string
		mime string
	)
	if err := s.db.QueryRowContext(r.Context(), `SELECT path, IFNULL(mime,'application/octet-stream') FROM tracks WHERE id=?`, id).Scan(&path, &mime); err != nil {
		http.Error(w, "not found", 404)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", 404)
		} else {
			http.Error(w, "open", 500)
		}
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
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
