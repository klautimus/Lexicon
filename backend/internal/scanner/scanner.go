package scanner

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

type Scanner struct {
	db *sql.DB
}

func New(db *sql.DB) *Scanner { return &Scanner{db: db} }

var audioExts = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".m4b":  "audio/mp4",
	".aac":  "audio/aac",
	".ogg":  "audio/ogg",
	".opus": "audio/opus",
	".wav":  "audio/wav",
}

func (s *Scanner) ScanRoot(ctx context.Context, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		mime, ok := audioExts[ext]
		if !ok {
			return nil
		}
		return s.indexFile(ctx, path, mime)
	})
}

func (s *Scanner) indexFile(ctx context.Context, path, mime string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mtime := info.ModTime().Unix()

	var existingMtime sql.NullInt64
	_ = s.db.QueryRowContext(ctx, "SELECT mtime FROM tracks WHERE path=?", path).Scan(&existingMtime)
	if existingMtime.Valid && existingMtime.Int64 == mtime {
		return nil // up-to-date
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var (
		title, artist, albumArtist, album, genre string
		trackNo, discNo, year                    int
	)
	if m, err := tag.ReadFrom(f); err == nil {
		title = m.Title()
		artist = m.Artist()
		albumArtist = m.AlbumArtist()
		album = m.Album()
		genre = m.Genre()
		trackNo, _ = m.Track()
		discNo, _ = m.Disc()
		year = m.Year()
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	kind := "music"
	g := strings.ToLower(genre)
	if strings.Contains(g, "podcast") || strings.Contains(strings.ToLower(path), "podcast") {
		kind = "podcast"
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracks(path,title,artist,album_artist,album,track_no,disc_no,year,genre,mime,size_bytes,media_kind,mtime)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			title=excluded.title, artist=excluded.artist, album_artist=excluded.album_artist,
			album=excluded.album, track_no=excluded.track_no, disc_no=excluded.disc_no,
			year=excluded.year, genre=excluded.genre, mime=excluded.mime,
			size_bytes=excluded.size_bytes, media_kind=excluded.media_kind, mtime=excluded.mtime
	`, path, title, artist, albumArtist, album, trackNo, discNo, year, genre, mime, info.Size(), kind, mtime)
	return err
}
