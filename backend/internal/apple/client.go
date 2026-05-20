package apple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase = "https://api.music.apple.com/v1"
)

// ErrUnauthorized indicates the developer token or Music User Token was
// rejected. Callers (e.g. the syncer) should treat this as "user must
// re-authorize" and not retry endlessly.
var ErrUnauthorized = errors.New("apple music auth rejected")

// doRequest performs a GET against apiBase+path with the developer token in
// Authorization and (optionally) the Music User Token. It decodes the response
// into v. Honors Retry-After on 429 with one retry.
func doRequest(ctx context.Context, devTok, mut, path string, v any) error {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+devTok)
	if mut != "" {
		req.Header.Set("Music-User-Token", mut)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("apple music http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		// Retry once after Retry-After (max 10s)
		wait := 2 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if n, err := strconv.Atoi(ra); err == nil && n > 0 && n <= 10 {
				wait = time.Duration(n) * time.Second
			}
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
		req2, _ := http.NewRequestWithContext(ctx, "GET", apiBase+path, nil)
		req2.Header = req.Header.Clone()
		resp2, err := client.Do(req2)
		if err != nil {
			return fmt.Errorf("apple music http (retry): %w", err)
		}
		defer resp2.Body.Close()
		resp = resp2 //nolint:ineffassign // we use resp below via defer-shadow; reassign for clarity
		body, _ := io.ReadAll(resp2.Body)
		if resp2.StatusCode >= 400 {
			if resp2.StatusCode == 401 || resp2.StatusCode == 403 {
				return fmt.Errorf("%w: %s", ErrUnauthorized, snippet(body))
			}
			return fmt.Errorf("apple music %d: %s", resp2.StatusCode, snippet(body))
		}
		return json.Unmarshal(body, v)
	}

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("%w: %s", ErrUnauthorized, snippet(body))
		}
		return fmt.Errorf("apple music %d %s: %s", resp.StatusCode, path, snippet(body))
	}
	return json.Unmarshal(body, v)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// -----------------------------------------------------------------------
// Response types (Apple's JSON:API-ish shape: {data: [{id, type, attributes}]})
// -----------------------------------------------------------------------

type SongAttributes struct {
	Name             string   `json:"name"`
	ArtistName       string   `json:"artistName"`
	AlbumName        string   `json:"albumName"`
	GenreNames       []string `json:"genreNames"`
	DurationInMillis int64    `json:"durationInMillis"`
	ReleaseDate      string   `json:"releaseDate"`
	URL              string   `json:"url"`
	ISRC             string   `json:"isrc"`
	TrackNumber      int      `json:"trackNumber"`
	DiscNumber       int      `json:"discNumber"`
	PlayParams       struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		CatalogID string `json:"catalogId"`
	} `json:"playParams"`
}

type SongResource struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Href       string         `json:"href"`
	Attributes SongAttributes `json:"attributes"`
}

type PlaylistAttributes struct {
	Name        string `json:"name"`
	Description struct {
		Standard string `json:"standard"`
		Short    string `json:"short"`
	} `json:"description"`
	CanEdit          bool   `json:"canEdit"`
	IsPublic         bool   `json:"isPublic"`
	LastModifiedDate string `json:"lastModifiedDate"`
	PlayParams       struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	} `json:"playParams"`
}

type PlaylistResource struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Href       string             `json:"href"`
	Attributes PlaylistAttributes `json:"attributes"`
}

type ArtistAttributes struct {
	Name       string   `json:"name"`
	GenreNames []string `json:"genreNames"`
	URL        string   `json:"url"`
}

type ArtistResource struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Attributes ArtistAttributes `json:"attributes"`
}

type ListResponse[T any] struct {
	Data []T    `json:"data"`
	Next string `json:"next"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

// StorefrontResponse models /v1/me/storefront.
type StorefrontResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Name                string `json:"name"`
			DefaultLanguageTag  string `json:"defaultLanguageTag"`
			SupportedLanguageTags []string `json:"supportedLanguageTags"`
		} `json:"attributes"`
	} `json:"data"`
}

// -----------------------------------------------------------------------
// Public fetchers
// -----------------------------------------------------------------------

// FetchUserStorefront returns the user's storefront ID (e.g. "us"). Used at
// connect-time to record where their catalog lives.
func FetchUserStorefront(ctx context.Context, devTok, mut string) (string, error) {
	var r StorefrontResponse
	if err := doRequest(ctx, devTok, mut, "/me/storefront", &r); err != nil {
		return "", err
	}
	if len(r.Data) == 0 {
		return "", errors.New("empty storefront response")
	}
	return r.Data[0].ID, nil
}

// FetchRecentlyPlayed returns the user's recently played tracks (max 30).
// Apple does not support an `after=<timestamp>` cursor here; we filter
// client-side against last_synced_at in the syncer.
func FetchRecentlyPlayed(ctx context.Context, devTok, mut string, limit int) (*ListResponse[SongResource], error) {
	if limit <= 0 || limit > 30 {
		limit = 30
	}
	var r ListResponse[SongResource]
	path := fmt.Sprintf("/me/recent/played/tracks?limit=%d", limit)
	if err := doRequest(ctx, devTok, mut, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchHeavyRotation returns the user's heavy-rotation items (mixed song /
// album / playlist / artist types — we read .Type to disambiguate).
func FetchHeavyRotation(ctx context.Context, devTok, mut string, limit int) (*ListResponse[SongResource], error) {
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	var r ListResponse[SongResource]
	path := fmt.Sprintf("/me/history/heavy-rotation?limit=%d", limit)
	if err := doRequest(ctx, devTok, mut, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchLibrarySongs returns the user's library songs (paginated; we fetch
// the first `limit` for the LLM enrichment summary).
func FetchLibrarySongs(ctx context.Context, devTok, mut string, limit int) (*ListResponse[SongResource], error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var r ListResponse[SongResource]
	path := fmt.Sprintf("/me/library/songs?limit=%d", limit)
	if err := doRequest(ctx, devTok, mut, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchLibraryPlaylists returns the user's library playlists.
func FetchLibraryPlaylists(ctx context.Context, devTok, mut string, limit int) (*ListResponse[PlaylistResource], error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var r ListResponse[PlaylistResource]
	path := fmt.Sprintf("/me/library/playlists?limit=%d", limit)
	if err := doRequest(ctx, devTok, mut, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchLibraryArtists returns artists in the user's library.
func FetchLibraryArtists(ctx context.Context, devTok, mut string, limit int) (*ListResponse[ArtistResource], error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var r ListResponse[ArtistResource]
	path := fmt.Sprintf("/me/library/artists?limit=%d", limit)
	if err := doRequest(ctx, devTok, mut, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// RecommendationsResponse models /v1/me/recommendations — Apple's editorial
// + algorithmic mixes. We surface the names as inspiration in the LLM prompt.
type RecommendationAttributes struct {
	Title struct {
		StringForDisplay string `json:"stringForDisplay"`
	} `json:"title"`
	Reason struct {
		StringForDisplay string `json:"stringForDisplay"`
	} `json:"reason"`
	Kind         string `json:"kind"`
	NextUpdateDate string `json:"nextUpdateDate"`
}

type RecommendationResource struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Attributes RecommendationAttributes `json:"attributes"`
}

// FetchRecommendations returns Apple's personalized recommendation buckets.
func FetchRecommendations(ctx context.Context, devTok, mut string, limit int) (*ListResponse[RecommendationResource], error) {
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	var r ListResponse[RecommendationResource]
	path := fmt.Sprintf("/me/recommendations?limit=%d", limit)
	if err := doRequest(ctx, devTok, mut, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchCatalogSong looks up a song in the catalog (no MUT required). Used to
// resolve genres/artwork for plays we ingest from the user's recent history.
func FetchCatalogSong(ctx context.Context, devTok, storefront, songID string) (*SongResource, error) {
	if storefront == "" {
		storefront = "us"
	}
	path := fmt.Sprintf("/catalog/%s/songs/%s", url.PathEscape(storefront), url.PathEscape(songID))
	var r ListResponse[SongResource]
	if err := doRequest(ctx, devTok, "", path, &r); err != nil {
		return nil, err
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("catalog song %s not found", songID)
	}
	s := r.Data[0]
	return &s, nil
}

// TopArtistTally is the synthesized "top artists" result combining heavy
// rotation, recently played, and library counts. The recommender uses this
// in lieu of Spotify's first-class top-artists endpoint (which Apple lacks).
type TopArtistTally struct {
	Name   string
	Genres []string
	Score  int // higher = stronger signal
}

// SynthesizeTopArtists combines heavy-rotation, recently-played, and library
// artists into a unified weighted list. Heavy rotation counts 3x; recently
// played 2x; library 1x. Caller usually wants top ~10.
func SynthesizeTopArtists(ctx context.Context, devTok, mut string, limit int) ([]TopArtistTally, error) {
	tally := map[string]*TopArtistTally{}
	add := func(name string, genres []string, weight int) {
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		t, ok := tally[key]
		if !ok {
			t = &TopArtistTally{Name: name, Genres: genres}
			tally[key] = t
		}
		t.Score += weight
		if len(t.Genres) == 0 && len(genres) > 0 {
			t.Genres = genres
		}
	}

	hr, err := FetchHeavyRotation(ctx, devTok, mut, 20)
	if err == nil {
		for _, s := range hr.Data {
			add(s.Attributes.ArtistName, s.Attributes.GenreNames, 3)
		}
	}
	rp, err := FetchRecentlyPlayed(ctx, devTok, mut, 30)
	if err == nil {
		for _, s := range rp.Data {
			add(s.Attributes.ArtistName, s.Attributes.GenreNames, 2)
		}
	}
	la, err := FetchLibraryArtists(ctx, devTok, mut, 100)
	if err == nil {
		for _, a := range la.Data {
			add(a.Attributes.Name, a.Attributes.GenreNames, 1)
		}
	}

	out := make([]TopArtistTally, 0, len(tally))
	for _, v := range tally {
		out = append(out, *v)
	}
	// Sort by score desc; stable enough using a simple insertion sort.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Score < out[j].Score; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
