package scanner

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

type Scanner struct {
	db        *sql.DB
	loudnessSem chan struct{} // semaphore limiting concurrent ffmpeg loudness measurements
}

func New(db *sql.DB) *Scanner {
	return &Scanner{
		db:          db,
		loudnessSem: make(chan struct{}, 8), // max 8 concurrent ffmpeg loudness measurements
	}
}

// loudnessResult holds parsed output from ffmpeg's loudnorm filter.
type loudnessResult struct {
	InputI  float64 // integrated loudness
	InputTP float64 // true peak
	InputLRA float64 // loudness range
}

// measureLoudness runs ffmpeg with loudnorm in measurement mode (I=-16 reference,
// print_format=json) and parses the input_* fields from the last frame of JSON
// output on stderr. Returns zero values if ffmpeg is unavailable or parsing fails.
// The caller provides a context; a 30-second timeout is applied internally.
func measureLoudness(ctx context.Context, path string) loudnessResult {
	// Timeout MUST be created BEFORE exec.CommandContext so the command is bound
	// to the timed context. Creating it after does nothing — the command would
	// use the original (unlimited) context and ffmpeg could hang indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", path,
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json",
		"-f", "null", "-",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// ffmpeg may return non-zero (e.g. early stream end) but still prints JSON; ignore err
		log.Printf("[scanner] loudness ffmpeg failed for %s: %v", path, err)
	}

	// Parse JSON frames from stderr — each line may be a log frame or JSON object
	var last loudnessResult
	for _, line := range bytes.Split(stderr.Bytes(), []byte{'\n'}) {
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var frame struct {
			InputI   float64 `json:"input_i"`
			InputTP  float64 `json:"input_tp"`
			InputLRA float64 `json:"input_lra"`
		}
		if json.Unmarshal(line, &frame) == nil && frame.InputI != 0 {
			last = loudnessResult{InputI: frame.InputI, InputTP: frame.InputTP, InputLRA: frame.InputLRA}
		}
	}
	return last
}

var audioExts = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".m4b":  "audio/mp4",
	".aac":  "audio/aac",
	".ogg":  "audio/ogg",
	".opus": "audio/opus",
	".wav":  "audio/wav",
	".mp4":  "audio/mp4",
	".webm": "audio/webm",
}

func (s *Scanner) ScanRoot(ctx context.Context, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("[scanner] walk error at %s: %v", path, err)
			return nil // skip errors but log them
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

	// Skip files that are too small to be valid audio (< 10KB)
	if info.Size() < 10240 {
		log.Printf("[scanner] skipping suspiciously small file: %s (%d bytes)", path, info.Size())
		return nil
	}

	var existingMtime sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT mtime FROM tracks WHERE path=?", path).Scan(&existingMtime); err != nil && err != sql.ErrNoRows {
		log.Printf("[scanner] failed to query existing mtime for %s: %v", path, err)
	}
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

	// Classify as music or podcast based on genre or path
	kind := "music"
	g := strings.ToLower(genre)
	if strings.Contains(g, "podcast") || strings.Contains(strings.ToLower(path), "podcast") {
		kind = "podcast"
	}

	// Insert/update the track in the DB first (fast path — no blocking ffmpeg call)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracks(path,title,artist,album_artist,album,track_no,disc_no,year,genre,mime,size_bytes,cover_path,added_at,media_kind,mtime,loudness_integrated,loudness_true_peak,loudness_range)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			title=excluded.title, artist=excluded.artist, album_artist=excluded.album_artist,
			album=excluded.album, track_no=excluded.track_no, disc_no=excluded.disc_no,
			year=excluded.year, genre=excluded.genre, mime=excluded.mime,
			size_bytes=excluded.size_bytes, cover_path=excluded.cover_path, media_kind=excluded.media_kind, mtime=excluded.mtime,
			loudness_integrated=excluded.loudness_integrated, loudness_true_peak=excluded.loudness_true_peak, loudness_range=excluded.loudness_range
	`, path, title, artist, albumArtist, album, trackNo, discNo, year, genre, mime, info.Size(), "", 0, kind, mtime, 0.0, 0.0, 0.0)
	if err != nil {
		return err
	}

	// Measure loudness asynchronously so it doesn't block the scan pipeline.
	// The result is written back to the DB when ready.
	// A semaphore (loudnessSem) limits concurrent ffmpeg processes to prevent
	// resource exhaustion on large libraries.
	go func() {
		s.loudnessSem <- struct{}{}        // acquire
		defer func() { <-s.loudnessSem }() // release

		l := measureLoudness(ctx, path)
		if l.InputI != 0 {
			_, err := s.db.ExecContext(ctx,
				`UPDATE tracks SET loudness_integrated=?, loudness_true_peak=?, loudness_range=? WHERE path=?`,
				l.InputI, l.InputTP, l.InputLRA, path)
			if err != nil {
				log.Printf("[scanner] failed to update loudness for %s: %v", path, err)
			}
		}
	}()

	return nil
}
