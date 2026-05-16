package models

import (
	"database/sql"
	"strings"
)

// Track is the canonical Track type used throughout Lexicon.
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

// TrackCols matches the actual tracks table schema exactly.
// Column order must match ScanTrack's Scan argument order.
// Uses raw column names — NULLs are handled by sql.NullString in ScanTrack.
const TrackCols = `id, title, artist, album_artist, album, track_no, disc_no, year, genre, duration_sec, media_kind, mime, spotify_id, external_url`

// TrackColsAliased returns TrackCols with a table prefix for JOIN queries
// where column names might be ambiguous (e.g., tracks_fts has title, artist, etc.).
func TrackColsAliased(alias string) string {
	cols := []string{
		"id", "title", "artist", "album_artist", "album",
		"track_no", "disc_no", "year", "genre",
		"duration_sec", "media_kind", "mime",
		"spotify_id", "external_url",
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
	var title, artist, albumArtist, album, genre, mediaKind, mime sql.NullString
	var spotifyID, externalURL sql.NullString
	var trackNo, discNo, year, durationSec sql.NullInt64

	err := s.Scan(
		&t.ID,
		&title, &artist, &albumArtist, &album,
		&trackNo, &discNo, &year, &genre,
		&durationSec, &mediaKind, &mime,
		&spotifyID, &externalURL,
	)
	if err != nil {
		return t, err
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
	if spotifyID.Valid {
		t.SpotifyID = spotifyID.String
	}
	if externalURL.Valid {
		t.ExternalURL = externalURL.String
	}

	return t, nil
}
