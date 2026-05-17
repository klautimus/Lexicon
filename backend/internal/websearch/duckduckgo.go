package websearch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type ddgProvider struct{}

func (d *ddgProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	u := "https://lite.duckduckgo.com/lite/"
	form := url.Values{}
	form.Set("q", query)
	form.Set("kl", "us-en")

	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ddg status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	doc.Find(".result-link").Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok || href == "" {
			return
		}
		title := strings.TrimSpace(s.Text())
		// Try to find the snippet in the adjacent result snippet cell
		snippet := ""
		snippetSel := s.Parent().Parent().Find(".result-snippet")
		if snippetSel.Length() > 0 {
			snippet = strings.TrimSpace(snippetSel.Text())
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     href,
			Snippet: snippet,
		})
	})

	return results, nil
}
