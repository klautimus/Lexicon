package models

import "database/sql"

// Track is the canonical Track type used throughout Lexicon.
type Track struct {
	ID           int64   `json:"id"`
	Path         *string `json:"path"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	AlbumArtist  *string `json:"album_artist"`
	Album        string  `json:"album"`
	Year         int     `json:"year"`
	Genre        string  `json:"genre"`
	DurationSec  float64 `json:"duration_sec"`
	Mime         string  `json:"mime"`
	MediaKind    string  `json:"media_kind"`
	SpotifyID    *string `json:"spotify_id"`
	ExternalURL  *string `json:"external_url"`
	FileSize     *int64  `json:"file_size"`
	DiscNo       int     `json:"disc_no"`
	TrackNo      int     `json:"track_no"`
	Bitrate      int     `json:"bitrate"`
	SampleRate   int     `json:"sample_rate"`
	CreatedAt    int64   `json:"created_at"`
	ModifiedAt   *int64  `json:"modified_at"`
}

// TrackCols is the list of track columns in SELECT queries.
const TrackCols = `id, path, title, artist, album_artist, album, year, genre, duration_sec, mime, media_kind, spotify_id, external_url, file_size, disc_no, track_no, bitrate, sample_rate, created_at, modified_at`

// ScanTrack scans a Track from a row.
func ScanTrack(s interface {
	Scan(dest ...interface{}) error
}) (Track, error) {
	var t Track
	var path, albumArtist, spotifyID, externalURL sql.NullString
	var fileSize, modifiedAt sql.NullInt64

	err := s.Scan(
		&t.ID, &path, &t.Title, &t.Artist, &albumArtist, &t.Album,
		&t.Year, &t.Genre, &t.DurationSec, &t.Mime, &t.MediaKind,
		&spotifyID, &externalURL, &fileSize, &t.DiscNo, &t.TrackNo,
		&t.Bitrate, &t.SampleRate, &t.CreatedAt, &modifiedAt,
	)
	if err != nil {
		return t, err
	}

	if path.Valid {
		t.Path = &path.String
	}
	if albumArtist.Valid {
		t.AlbumArtist = &albumArtist.String
	}
	if spotifyID.Valid {
		t.SpotifyID = &spotifyID.String
	}
	if externalURL.Valid {
		t.ExternalURL = &externalURL.String
	}
	if modifiedAt.Valid {
		t.ModifiedAt = &modifiedAt.Int64
	}
	if fileSize.Valid {
		t.FileSize = &fileSize.Int64
	}

	return t, nil
}
