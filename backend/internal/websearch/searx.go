package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

type searxProvider struct {
	instances []string
}

func (s *searxProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	// Rotating list of known public SearXNG instances
	if s.instances == nil {
		s.instances = []string{
			"https://search.bus-hit.me",
			"https://search.sapti.me",
			"https://search.mdosch.de",
			"https://searx.daetalytica.io",
			"https://searx.mha.fi",
		}
		rand.Shuffle(len(s.instances), func(i, j int) {
			s.instances[i], s.instances[j] = s.instances[j], s.instances[i]
		})
	}

	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("safesearch", "0")

	for _, base := range s.instances {
		u := base + "/search?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		body := make([]byte, 0, 64*1024)
		// Read up to 1MB
		buf := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if len(body)+n > 1024*1024 {
					break
				}
				body = append(body, buf[:n]...)
			}
			if err != nil || n == 0 {
				break
			}
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			continue
		}

		var parsed struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			continue
		}

		var out []SearchResult
		for _, r := range parsed.Results {
			if r.URL == "" {
				continue
			}
			out = append(out, SearchResult{
				Title:   r.Title,
				URL:     r.URL,
				Snippet: r.Content,
			})
			if len(out) >= 5 {
				break
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("all searx instances failed")
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
