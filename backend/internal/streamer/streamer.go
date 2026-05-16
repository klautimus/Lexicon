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
		http.Error(w, "open", 500)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "stat", 500)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}
