package websearch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TrackInfo holds a track discovered via web search.
type TrackInfo struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album,omitempty"`
}

// SearchResult is a single search-engine result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// ExtractedPage is the cleaned text extracted from a result page.
type ExtractedPage struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Text    string `json:"text"`
	IsMusic bool   `json:"is_music"`
}

// SearchProvider abstracts a search engine.
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// WebSearch is the main API for searching and extracting web content.
type WebSearch struct {
	providers []SearchProvider
	http      *http.Client
	cacheMu   sync.RWMutex
	cache     map[string]cacheEntry
	enabled   bool
}

type cacheEntry struct {
	results []ExtractedPage
	at      time.Time
}

// New creates a WebSearch instance.
func New(enabled bool) *WebSearch {
	return &WebSearch{
		providers: []SearchProvider{
			&ddgProvider{},
			&searxProvider{},
		},
		http:    &http.Client{Timeout: 15 * time.Second},
		cache:   make(map[string]cacheEntry),
		enabled: enabled,
	}
}

// SearchAndExtract searches the web and extracts text from top result pages.
func (w *WebSearch) SearchAndExtract(ctx context.Context, query string) ([]ExtractedPage, error) {
	if !w.enabled {
		return nil, nil
	}
	key := cacheKey(query)
	w.cacheMu.RLock()
	ent, ok := w.cache[key]
	w.cacheMu.RUnlock()
	if ok && time.Since(ent.at) < 5*time.Minute {
		return ent.results, nil
	}

	var results []SearchResult
	for _, p := range w.providers {
		r, err := p.Search(ctx, query)
		if err != nil {
			log.Printf("[websearch] provider %T error: %v", p, err)
			continue
		}
		results = append(results, r...)
		if len(results) >= 3 {
			break
		}
	}

	pages := w.fetchAndExtract(ctx, results)
	w.cacheMu.Lock()
	w.cache[key] = cacheEntry{results: pages, at: time.Now()}
	w.cacheMu.Unlock()
	return pages, nil
}

// ResolveAlbumTracks tries to find the track listing for an album via web search.
func (w *WebSearch) ResolveAlbumTracks(ctx context.Context, artist, album string) ([]TrackInfo, error) {
	query := fmt.Sprintf("%s %s tracklist", artist, album)
	pages, err := w.SearchAndExtract(ctx, query)
	if err != nil || len(pages) == 0 {
		return nil, err
	}
	for _, p := range pages {
		tracks := extractTrackList(p.Text)
		if len(tracks) > 0 {
			for i := range tracks {
				tracks[i].Artist = artist
				tracks[i].Album = album
			}
			return tracks, nil
		}
	}
	return nil, nil
}

// ResolveLatestAlbumTracks finds the latest album by an artist and returns its tracks.
func (w *WebSearch) ResolveLatestAlbumTracks(ctx context.Context, artist string) ([]TrackInfo, error) {
	query := fmt.Sprintf("%s latest album new album tracklist", artist)
	pages, err := w.SearchAndExtract(ctx, query)
	if err != nil || len(pages) == 0 {
		return nil, err
	}
	for _, p := range pages {
		tracks := extractTrackList(p.Text)
		if len(tracks) > 0 {
			for i := range tracks {
				tracks[i].Artist = artist
			}
			return tracks, nil
		}
	}
	return nil, nil
}

// DetectSearchQuery analyses a user message and returns a search query if it looks
// like a request for album/artist/latest-release tracks.
func DetectSearchQuery(msg string) string {
	msg = strings.ToLower(msg)
	// Patterns: "new album by X", "latest album by X", "tracks from X album Y",
	// "album X by Y tracklist", "songs from X's new album"
	patterns := []struct {
		re    *regexp.Regexp
		group int // which capture group contains the artist/album query
	}{
		// "new album by Kendrick Lamar"
		{regexp.MustCompile(`(?:new|latest)\s+(?:album|ep|release)\s+(?:by|from)\s+(.+?)(?:tracklist|songs|playlist|$)`), 1},
		// "Kendrick Lamar new album"
		{regexp.MustCompile(`(.+?)\s+(?:new|latest)\s+(?:album|ep|release)`), 1},
		// "tracks from GNX by Kendrick Lamar"
		{regexp.MustCompile(`(?:tracks?|songs?)\s+(?:from|on|off)\s+(.+?)(?:tracklist|playlist|$)`), 1},
		// "Kendrick Lamar GNX tracklist"
		{regexp.MustCompile(`(.+?)\s+(?:tracklist|track listing|songs|playlist)`), 1},
		// "make a playlist from Kendrick Lamar's new album"
		{regexp.MustCompile(`(?:make|create|generate)\s+(?:a\s+)?(?:playlist|mix)\s+(?:from|with|of)\s+(.+?)$`), 1},
	}
	for _, p := range patterns {
		if m := p.re.FindStringSubmatch(msg); m != nil && len(m) > p.group {
			q := strings.TrimSpace(m[p.group])
			if q != "" {
				return q + " tracklist"
			}
		}
	}
	return ""
}

func (w *WebSearch) fetchAndExtract(ctx context.Context, results []SearchResult) []ExtractedPage {
	var wg sync.WaitGroup
	ch := make(chan ExtractedPage, len(results))
	sem := make(chan struct{}, 3)
	for _, r := range results {
		if !looksLikeMusicPage(r.URL) {
			continue
		}
		wg.Add(1)
		go func(res SearchResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			page, err := fetchAndExtractPage(ctx, w.http, res.URL)
			if err != nil {
				return
			}
			page.Title = res.Title
			ch <- page
		}(r)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	var pages []ExtractedPage
	for p := range ch {
		pages = append(pages, p)
	}
	return pages
}

func cacheKey(q string) string {
	h := sha256.Sum256([]byte(q))
	return fmt.Sprintf("%x", h)[:16]
}

func looksLikeMusicPage(url string) bool {
	// Skip social media, shopping, video sites
	bad := []string{"youtube.com", "youtu.be", "spotify.com", "apple.com/music",
		"amazon.com", "ebay.com", "facebook.com", "twitter.com", "x.com",
		"instagram.com", "tiktok.com", "reddit.com"}
	u := strings.ToLower(url)
	for _, b := range bad {
		if strings.Contains(u, b) {
			return false
		}
	}
	return true
}
