package models

import (
	"database/sql"
	"fmt"
	"strings"
)

// Track is the canonical Track type used throughout Lexicon.
type Track struct {
	ID                int64   `json:"id"`
	Path              string  `json:"path"`
	Title             string  `json:"title"`
	Artist            string  `json:"artist"`
	AlbumArtist       string  `json:"album_artist"`
	Album             string  `json:"album"`
	TrackNo           int     `json:"track_no"`
	DiscNo            int     `json:"disc_no"`
	Year              int     `json:"year"`
	Genre             string  `json:"genre"`
	DurationSec       int     `json:"duration_sec"`
	MediaKind         string  `json:"media_kind"`
	Mime              string  `json:"mime"`
	SizeBytes         int64   `json:"size_bytes"`
	CoverPath         string  `json:"cover_path,omitempty"`
	AddedAt           int64   `json:"added_at"`
	Mtime             int64   `json:"mtime"`
	// Loudness fields use omitempty: 0.0 values are excluded from JSON.
	// This is correct when the DB value is NULL (not yet measured), but
	// technically ambiguous for true-zero measurements (~0 LUFS, rare in practice).
	// See BUG-LOGIC-21 in audit-fix-plan-2026-05-20.md.
	LoudnessIntegrated float64 `json:"loudness_integrated,omitempty"`
	LoudnessTruePeak  float64 `json:"loudness_true_peak,omitempty"`
	LoudnessRange     float64 `json:"loudness_range,omitempty"`
	SpotifyID         string  `json:"spotify_id,omitempty"`
	ExternalURL       string  `json:"external_url,omitempty"`
	AppleID           string  `json:"apple_id,omitempty"`
}

// TrackCols matches the actual tracks table schema exactly.
// Column order must match ScanTrack's Scan argument order.
// Uses raw column names — NULLs are handled by sql.NullString in ScanTrack.
const TrackCols = `id, path, title, artist, album_artist, album, track_no, disc_no, year, genre, duration_sec, media_kind, mime, size_bytes, cover_path, added_at, mtime, loudness_integrated, loudness_true_peak, loudness_range, spotify_id, external_url, apple_id`

// ExpectedTrackCols is the exact number of columns in TrackCols.
// Must match the dest argument count in ScanTrack and the columns in
// TrackColsAliased. Edit all three when adding or removing columns.
const ExpectedTrackCols = 23

// TrackColsAliased returns TrackCols with a table prefix for JOIN queries
// where column names might be ambiguous (e.g., tracks_fts has title, artist, etc.).
func TrackColsAliased(alias string) string {
	cols := []string{
		"id", "path", "title", "artist", "album_artist", "album",
		"track_no", "disc_no", "year", "genre",
		"duration_sec", "media_kind", "mime",
		"size_bytes", "cover_path", "added_at", "mtime",
		"loudness_integrated", "loudness_true_peak", "loudness_range",
		"spotify_id", "external_url", "apple_id",
	}
	for i, c := range cols {
		cols[i] = alias + "." + c
	}
	return strings.Join(cols, ", ")
}

// ScanTrack scans a Track from a database row. The row must have been
// SELECTed with TrackCols (column order must match the Scan args below).
func ScanTrack(s interface {
	Scan(dest ...interface{}) error
}) (Track, error) {
	var t Track
	var path, title, artist, albumArtist, album, genre, mediaKind, mime sql.NullString
	var spotifyID, externalURL, appleID sql.NullString
	var trackNo, discNo, year, durationSec sql.NullInt64
	var sizeBytes, addedAt, mtime sql.NullInt64
	var coverPath sql.NullString
	var loudnessIntegrated, loudnessTruePeak, loudnessRange sql.NullFloat64

	dests := []interface{}{
		&t.ID,
		&path, &title, &artist, &albumArtist, &album,
		&trackNo, &discNo, &year, &genre,
		&durationSec, &mediaKind, &mime,
		&sizeBytes, &coverPath, &addedAt, &mtime,
		&loudnessIntegrated, &loudnessTruePeak, &loudnessRange,
		&spotifyID, &externalURL, &appleID,
	}

	if len(dests) != ExpectedTrackCols {
		return t, fmt.Errorf("ScanTrack: dest count %d != ExpectedTrackCols %d — update ScanTrack when columns change", len(dests), ExpectedTrackCols)
	}

	err := s.Scan(dests...)
	if err != nil {
		return t, err
	}

	if path.Valid {
		t.Path = path.String
	}
	if title.Valid {
		t.Title = title.String
	}
	if artist.Valid {
		t.Artist = artist.String
	}
	if albumArtist.Valid {
		t.AlbumArtist = albumArtist.String
	}
	if album.Valid {
		t.Album = album.String
	}
	if trackNo.Valid {
		t.TrackNo = int(trackNo.Int64)
	}
	if discNo.Valid {
		t.DiscNo = int(discNo.Int64)
	}
	if year.Valid {
		t.Year = int(year.Int64)
	}
	if genre.Valid {
		t.Genre = genre.String
	}
	if durationSec.Valid {
		t.DurationSec = int(durationSec.Int64)
	}
	if mediaKind.Valid {
		t.MediaKind = mediaKind.String
	}
	if mime.Valid {
		t.Mime = mime.String
	}
	if sizeBytes.Valid {
		t.SizeBytes = sizeBytes.Int64
	}
	if coverPath.Valid {
		t.CoverPath = coverPath.String
	}
	if addedAt.Valid {
		t.AddedAt = addedAt.Int64
	}
	if mtime.Valid {
		t.Mtime = mtime.Int64
	}
	if loudnessIntegrated.Valid {
		t.LoudnessIntegrated = loudnessIntegrated.Float64
	}
	if loudnessTruePeak.Valid {
		t.LoudnessTruePeak = loudnessTruePeak.Float64
	}
	if loudnessRange.Valid {
		t.LoudnessRange = loudnessRange.Float64
	}
	if spotifyID.Valid {
		t.SpotifyID = spotifyID.String
	}
	if externalURL.Valid {
		t.ExternalURL = externalURL.String
	}
	if appleID.Valid {
		t.AppleID = appleID.String
	}

	return t, nil
}
