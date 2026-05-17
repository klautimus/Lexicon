package spotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// validAccessToken returns a non-expired access token, refreshing if necessary.
func (a *API) validAccessToken(ctx context.Context) (string, error) {
	return ensureToken(ctx, a.db, a.cfg.ClientID)
}

func ensureToken(ctx context.Context, db *sql.DB, clientID string) (string, error) {
	var (
		access, refresh string
		expiresAt       int64
	)
	err := db.QueryRowContext(ctx,
		`SELECT access_token, refresh_token, expires_at FROM spotify_tokens WHERE id=1`).
		Scan(&access, &refresh, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("not connected: %w", err)
	}
	// Refresh if expiring within 60s
	if time.Now().Unix() >= expiresAt-60 {
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refresh)
		form.Set("client_id", clientID)
		tr, err := postToken(ctx, form)
		if err != nil {
			return "", fmt.Errorf("refresh failed: %w", err)
		}
		newRefresh := refresh
		if tr.RefreshToken != "" {
			newRefresh = tr.RefreshToken
		}
		newExpires := time.Now().Unix() + int64(tr.ExpiresIn)
		_, err = db.ExecContext(ctx, `
			UPDATE spotify_tokens SET access_token=?, refresh_token=?, expires_at=?, scope=?
			WHERE id=1`, tr.AccessToken, newRefresh, newExpires, tr.Scope)
		if err != nil {
			return "", err
		}
		return tr.AccessToken, nil
	}
	return access, nil
}

func spotifyGET(ctx context.Context, accessToken, path string, q url.Values) (*http.Response, error) {
	u := apiBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 429 {
		ra := resp.Header.Get("Retry-After")
		secs, _ := strconv.Atoi(ra)
		if secs <= 0 {
			secs = 1
		}
		resp.Body.Close()
		select {
		case <-time.After(time.Duration(secs) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return spotifyGET(ctx, accessToken, path, q)
	}
	return resp, nil
}

// ----- Recently played -----

type RecentlyPlayedResponse struct {
	Items  []RecentItem `json:"items"`
	Cursors struct {
		After  string `json:"after"`
		Before string `json:"before"`
	} `json:"cursors"`
}

type RecentItem struct {
	PlayedAt string      `json:"played_at"` // RFC3339 timestamp
	Track    SpotifyTrack `json:"track"`
}

type SpotifyTrack struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DurationMs  int             `json:"duration_ms"`
	URI         string          `json:"uri"`
	ExternalURL map[string]string `json:"external_urls"`
	Album       struct {
		Name        string `json:"name"`
		ReleaseDate string `json:"release_date"`
	} `json:"album"`
	Artists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artists"`
}

func recentlyPlayed(ctx context.Context, accessToken string, afterMs int64) (*RecentlyPlayedResponse, error) {
	q := url.Values{}
	q.Set("limit", "50")
	if afterMs > 0 {
		q.Set("after", strconv.FormatInt(afterMs, 10))
	}
	resp, err := spotifyGET(ctx, accessToken, "/me/player/recently-played", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("recently-played %d: %s", resp.StatusCode, string(body))
	}
	var out RecentlyPlayedResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func artistsString(t SpotifyTrack) string {
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

func releaseYear(date string) int {
	if len(date) >= 4 {
		y, _ := strconv.Atoi(date[:4])
		return y
	}
	return 0
}

func fetchArtistGenres(ctx context.Context, accessToken string, artistIDs []string) (map[string]string, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}
	genreMap := make(map[string]string)
	for _, id := range artistIDs {
		resp, err := spotifyGET(ctx, accessToken, "/artists/"+id, nil)
		if err != nil {
			log.Printf("[spotify] fetch artist %s: %v", id, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("[spotify] fetch artist %s: HTTP %d", id, resp.StatusCode)
			continue
		}
		var artist struct {
			ID     string   `json:"id"`
			Genres []string `json:"genres"`
		}
		if err := json.Unmarshal(body, &artist); err != nil {
			log.Printf("[spotify] parse artist %s: %v", id, err)
			continue
		}
		if len(artist.Genres) > 0 {
			genreMap[artist.ID] = strings.Join(artist.Genres, ", ")
		}
	}
	return genreMap, nil
}
