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

// ValidAccessToken returns a non-expired access token, refreshing if necessary.
func (a *API) ValidAccessToken(ctx context.Context) (string, error) {
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
	// Batch in groups of 20 (Spotify API limit for /artists endpoint)
	for i := 0; i < len(artistIDs); i += 20 {
		end := i + 20
		if end > len(artistIDs) {
			end = len(artistIDs)
		}
		batch := artistIDs[i:end]
		ids := strings.Join(batch, ",")
		resp, err := spotifyGET(ctx, accessToken, "/artists?ids="+ids, nil)
		if err != nil {
			log.Printf("[spotify] fetch artists batch: %v", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("[spotify] fetch artists batch: HTTP %d", resp.StatusCode)
			continue
		}
		var result struct {
			Artists []struct {
				ID     string   `json:"id"`
				Genres []string `json:"genres"`
			} `json:"artists"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("[spotify] parse artists batch: %v", err)
			continue
		}
		for _, artist := range result.Artists {
			if len(artist.Genres) > 0 {
				genreMap[artist.ID] = strings.Join(artist.Genres, ", ")
			}
		}
	}
	return genreMap, nil
}

// SpotifyTopArtists represents a user's top artists from Spotify
type SpotifyTopArtists struct {
	Items []struct {
		Name    string `json:"name"`
		Genres  []string `json:"genres"`
		Images  []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"items"`
}

// SpotifyTopTracks represents a user's top tracks from Spotify
type SpotifyTopTracks struct {
	Items []struct {
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name string `json:"name"`
		} `json:"album"`
	} `json:"items"`
}

// FetchTopArtists fetches the user's top artists from Spotify
func FetchTopArtists(ctx context.Context, accessToken string, limit int) (*SpotifyTopArtists, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("time_range", "medium_term") // last 6 months
	resp, err := spotifyGET(ctx, accessToken, "/me/top/artists", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("top-artists %d: %s", resp.StatusCode, string(body))
	}
	var result SpotifyTopArtists
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FetchTopTracks fetches the user's top tracks from Spotify
func FetchTopTracks(ctx context.Context, accessToken string, limit int) (*SpotifyTopTracks, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("time_range", "medium_term")
	resp, err := spotifyGET(ctx, accessToken, "/me/top/tracks", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("top-tracks %d: %s", resp.StatusCode, string(body))
	}
	var result SpotifyTopTracks
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SpotifyPlaylist represents a user's playlist
type SpotifyPlaylist struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ID            string `json:"id"`
	Public        bool   `json:"public"`
	Collaborative bool   `json:"collaborative"`
	Tracks        struct {
		Total int `json:"total"`
	} `json:"tracks"`
}

// SpotifySavedTrack represents a track in the user's library
type SpotifySavedTrack struct {
	Track struct {
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name string `json:"name"`
		} `json:"album"`
	} `json:"track"`
	AddedAt string `json:"added_at"`
}

// SpotifyFollowedArtist represents an artist the user follows
type SpotifyFollowedArtist struct {
	Name   string   `json:"name"`
	Genres []string `json:"genres"`
}

// FetchUserPlaylists fetches the user's playlists
func FetchUserPlaylists(ctx context.Context, accessToken string, limit int) ([]SpotifyPlaylist, error) {
	if limit <= 0 {
		limit = 50
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	resp, err := spotifyGET(ctx, accessToken, "/me/playlists", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("user-playlists %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Items []SpotifyPlaylist `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

// FetchSavedTracks fetches a sample of the user's saved/liked tracks
func FetchSavedTracks(ctx context.Context, accessToken string, limit int) ([]SpotifySavedTrack, error) {
	if limit <= 0 {
		limit = 50
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	resp, err := spotifyGET(ctx, accessToken, "/me/tracks", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("saved-tracks %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Items []SpotifySavedTrack `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

// FetchFollowedArtists fetches artists the user follows
func FetchFollowedArtists(ctx context.Context, accessToken string, limit int) ([]SpotifyFollowedArtist, error) {
	if limit <= 0 {
		limit = 50
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("type", "artist")
	resp, err := spotifyGET(ctx, accessToken, "/me/following", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("followed-artists %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Artists struct {
			Items []SpotifyFollowedArtist `json:"items"`
		} `json:"artists"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Artists.Items, nil
}

// SpotifyDevice represents a Spotify Connect device
type SpotifyDevice struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	IsActive      bool   `json:"is_active"`
	IsRestricted  bool   `json:"is_restricted"`
	VolumePercent int    `json:"volume_percent"`
}

// SpotifyDevicesResponse is the API response for /me/player/devices
type SpotifyDevicesResponse struct {
	Devices []SpotifyDevice `json:"devices"`
}

// GetDevices returns all available Spotify Connect devices
func GetDevices(ctx context.Context, accessToken string) ([]SpotifyDevice, error) {
	resp, err := spotifyGET(ctx, accessToken, "/me/player/devices", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("devices %d: %s", resp.StatusCode, string(body))
	}
	var result SpotifyDevicesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Devices, nil
}

// TransferPlayback transfers playback to a specific device
func TransferPlayback(ctx context.Context, accessToken string, deviceID string, play bool) error {
	body := fmt.Sprintf(`{"device_ids":["%s"],"play":%t}`, deviceID, play)
	req, err := http.NewRequestWithContext(ctx, "PUT",
		"https://api.spotify.com/v1/me/player", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("transfer %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
