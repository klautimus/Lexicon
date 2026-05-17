package websearch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func fetchAndExtractPage(ctx context.Context, client *http.Client, pageURL string) (ExtractedPage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return ExtractedPage{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil {
		return ExtractedPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ExtractedPage{}, fmt.Errorf("http %d", resp.StatusCode)
	}

	// Limit read to 2MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return ExtractedPage{}, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return ExtractedPage{}, err
	}

	// Try to find the main content area
	var text string
	for _, sel := range []string{
		"article",
		"[role='main']",
		".main-content",
		"#main-content",
		"main",
		".post-content",
		".entry-content",
		".article-body",
		"#article-body",
		".content",
		"#content",
	} {
		if content := doc.Find(sel).First().Text(); len(content) > 200 {
			text = content
			break
		}
	}
	if text == "" {
		// Fallback: body minus nav/footer/script/style
		text = doc.Find("body").Text()
	}

	// Clean up whitespace
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	text = strings.Join(cleaned, "\n")

	isMusic := strings.Contains(strings.ToLower(text), "tracklist") ||
		strings.Contains(strings.ToLower(text), "track listing") ||
		strings.Contains(strings.ToLower(text), "songs") && strings.Contains(strings.ToLower(text), "album")

	return ExtractedPage{
		URL:     pageURL,
		Title:   doc.Find("title").First().Text(),
		Text:    text,
		IsMusic: isMusic,
	}, nil
}

// extractTrackList tries to parse track names from extracted page text.
func extractTrackList(text string) []TrackInfo {
	var tracks []TrackInfo
	lines := strings.Split(text, "\n")

	// Heuristic 1: numbered lines like "1. Song Name" or "1) Song Name"
	numberedRE := regexp.MustCompile(`^\s*(?:\d+)[.\)]\s*(.+)$`)
	// Heuristic 2: lines with duration "Song Name — 3:42" or "Song Name (3:42)"
	durationRE := regexp.MustCompile(`(.+?)\s*(?:[—–-]|\()\s*\d+[:\.]\d+`)

	consecutive := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip obvious non-track lines
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "advertisement") ||
			strings.HasPrefix(lower, "subscribe") ||
			strings.HasPrefix(lower, "related") ||
			strings.HasPrefix(lower, "more from") ||
			strings.HasPrefix(lower, "you may also") ||
			strings.HasPrefix(lower, "editor") ||
			strings.HasPrefix(lower, "credit") ||
			strings.HasPrefix(lower, "release date") ||
			strings.HasPrefix(lower, "genre") ||
			strings.HasPrefix(lower, "label") ||
			strings.HasPrefix(lower, "producer") ||
			len(line) > 120 {
			consecutive = 0
			continue
		}

		var title string
		if m := numberedRE.FindStringSubmatch(line); m != nil {
			title = strings.TrimSpace(m[1])
			consecutive++
		} else if m := durationRE.FindStringSubmatch(line); m != nil {
			title = strings.TrimSpace(m[1])
			consecutive++
		} else if consecutive > 0 && len(line) < 80 && !strings.Contains(line, ":") {
			// Continue track list pattern without numbers
			title = line
			consecutive++
		} else {
			consecutive = 0
			continue
		}

		// Clean up
		title = strings.Trim(title, `"'"`)
		if title == "" {
			continue
		}
		tracks = append(tracks, TrackInfo{Title: title})
	}

	// Require at least 3 tracks to consider it a valid tracklist
	if len(tracks) < 3 {
		return nil
	}
	return tracks
}
