package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizeURL = "https://accounts.spotify.com/authorize"
	tokenURL     = "https://accounts.spotify.com/api/token"
	apiBase      = "https://api.spotify.com/v1"

	scopes = "user-read-recently-played user-top-read user-read-currently-playing user-read-playback-state streaming user-read-email user-read-private user-library-read playlist-read-private playlist-read-collaborative user-follow-read"
)

func randB64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (a *API) authURL(w http.ResponseWriter, r *http.Request) {
	if !a.configured() {
		http.Error(w, "SPOTIFY_CLIENT_ID not set in backend/.env", 400)
		return
	}
	verifier, err := randB64(64)
	if err != nil {
		log.Printf("[spotify] authURL: randB64 verifier: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}
	state, err := randB64(24)
	if err != nil {
		log.Printf("[spotify] authURL: randB64 state: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}
	a.verifiers.Store(state, verifierEntry{Verifier: verifier, CreatedAt: time.Now()})

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", a.cfg.ClientID)
	q.Set("scope", scopes)
	q.Set("redirect_uri", a.cfg.RedirectURI)
	q.Set("state", state)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", pkceChallenge(verifier))

	http.Redirect(w, r, authorizeURL+"?"+q.Encode(), http.StatusFound)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (a *API) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errStr := q.Get("error"); errStr != "" {
		http.Redirect(w, r, a.cfg.FrontendURL+"/settings?spotify=error&reason="+url.QueryEscape(errStr), http.StatusFound)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		log.Printf("[spotify] callback: missing code/state")
		http.Error(w, "missing code/state", 400)
		return
	}
	var verifier string
	entry, ok := a.verifiers.Load(state)
	if !ok {
		log.Printf("[spotify] callback: invalid or expired state")
		http.Error(w, "invalid or expired state", 400)
		return
	}
	verifier = entry.(verifierEntry).Verifier
	a.verifiers.Delete(state)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", a.cfg.RedirectURI)
	form.Set("client_id", a.cfg.ClientID)
	form.Set("code_verifier", verifier)

	tok, err := postToken(r.Context(), form)
	if err != nil {
		log.Printf("[spotify] callback: token exchange: %v", err)
		http.Error(w, "token exchange failed: "+err.Error(), 500)
		return
	}

	// Pull /me for display name + product type
	me, err := fetchMe(r.Context(), tok.AccessToken)
	if err != nil {
		log.Printf("[spotify] callback: fetch /me: %v", err)
		http.Error(w, "fetch /me failed: "+err.Error(), 500)
		return
	}

	expiresAt := time.Now().Unix() + int64(tok.ExpiresIn)
	uid := userIDFromContext(r.Context())
	if _, err := a.db.ExecContext(r.Context(), `
		INSERT INTO spotify_tokens(lexicon_user_id, access_token, refresh_token, expires_at, scope, user_id, display_name, product, last_synced_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(lexicon_user_id) DO UPDATE SET
			access_token=excluded.access_token,
			refresh_token=excluded.refresh_token,
			expires_at=excluded.expires_at,
			scope=excluded.scope,
			user_id=excluded.user_id,
			display_name=excluded.display_name,
			product=excluded.product
	`, uid, tok.AccessToken, tok.RefreshToken, expiresAt, tok.Scope, me.ID, me.DisplayName, me.Product); err != nil {
		log.Printf("[spotify] callback: insert token: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Kick off an immediate sync in the background, detached from the
	// request context so the sync isn't cancelled when the HTTP response
	// is sent (the client gets a redirect immediately).
	if a.sync != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := a.sync.RunOnce(ctx); err != nil {
				log.Printf("[spotify] post-callback sync: %v", err)
			}
		}()
	}

	http.Redirect(w, r, a.cfg.FrontendURL+"/settings?spotify=ok", http.StatusFound)
}

func postToken(ctx context.Context, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

type spotifyMe struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Product     string `json:"product"`
	Email       string `json:"email"`
}

func fetchMe(ctx context.Context, accessToken string) (*spotifyMe, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var me spotifyMe
	if err := json.Unmarshal(body, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

type StatusResponse struct {
	Configured     bool   `json:"configured"`
	Connected      bool   `json:"connected"`
	DisplayName    string `json:"display_name,omitempty"`
	Product        string `json:"product,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	LastSyncedAt   int64  `json:"last_synced_at,omitempty"`
	HasPlaybackSDK bool   `json:"has_playback_sdk"`
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	st := StatusResponse{Configured: a.configured()}
	uid := userIDFromContext(r.Context())
	row := a.db.QueryRowContext(r.Context(),
		`SELECT IFNULL(display_name,''), IFNULL(product,''), IFNULL(user_id,''), last_synced_at FROM spotify_tokens WHERE lexicon_user_id=?`, uid)
	var ls int64
	if err := row.Scan(&st.DisplayName, &st.Product, &st.UserID, &ls); err == nil {
		st.Connected = true
		st.LastSyncedAt = ls
		st.HasPlaybackSDK = strings.EqualFold(st.Product, "premium")
	}
	writeJSON(w, st)
}

func (a *API) disconnect(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromContext(r.Context())
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM spotify_tokens WHERE lexicon_user_id=?`, uid); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
