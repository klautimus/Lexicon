// Package spotify implements PKCE OAuth, token storage, history sync,
// and Web Playback SDK token minting for Spotify integration.
package spotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/auth"
)

type verifierEntry struct {
	Verifier  string
	CreatedAt time.Time
}

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	FrontendURL  string
}

type API struct {
	db             *sql.DB
	cfg            Config
	sync           *Syncer
	verifiers      sync.Map
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

func New(db *sql.DB, cfg Config) *API {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	a := &API{db: db, cfg: cfg, shutdownCtx: shutdownCtx, shutdownCancel: shutdownCancel}
	a.sync = NewSyncer(db, cfg)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-a.shutdownCtx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-10 * time.Minute)
				a.verifiers.Range(func(key, value any) bool {
					if entry, ok := value.(verifierEntry); ok && entry.CreatedAt.Before(cutoff) {
						a.verifiers.Delete(key)
					}
					return true
				})
			}
		}
	}()
	return a
}

// Syncer returns the background syncer so main can start it.
func (a *API) Syncer() *Syncer { return a.sync }

// Shutdown signals the background syncer goroutine to cancel and exit.
// Call this before shutting down the HTTP server so that the syncer's
// goroutine observes the cancelled context and exits promptly.
func (a *API) Shutdown() {
	a.shutdownCancel()
}

// StartSyncer launches the background syncer goroutine using the API's
// shutdown context. Safe to call once at startup. No-op if called multiple
// times (the syncer goroutine is already running).
func (a *API) StartSyncer() {
	a.sync.Start(a.shutdownCtx)
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/spotify/auth-url", a.authURL)
	r.Get("/api/spotify/callback", a.callback)
	r.Get("/api/spotify/status", a.status)
	r.Post("/api/spotify/disconnect", a.disconnect)
	r.Post("/api/spotify/sync", a.manualSync)
	r.Get("/api/spotify/token", a.sdkToken)
	r.Get("/api/spotify/devices", a.devices)
	r.Post("/api/spotify/transfer", a.transfer)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[spotify] writeJSON encode: %v", err)
	}
}

func (a *API) configured() bool {
	return a.cfg.ClientID != ""
}

// userIDFromContext extracts the Lexicon user ID from the request context.
// Falls back to 1 (default admin) when no user is authenticated (desktop app).
func userIDFromContext(ctx context.Context) int64 {
	u, ok := auth.UserFromContext(ctx)
	if !ok || u == nil {
		return 1
	}
	return u.UserID
}

func (a *API) devices(w http.ResponseWriter, r *http.Request) {
	if !a.configured() {
		http.Error(w, "spotify not configured", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	token, err := a.ValidAccessToken(ctx)
	if err != nil {
		log.Printf("[spotify] devices: not connected: %v", err)
		http.Error(w, "not connected: "+err.Error(), 400)
		return
	}
	devs, err := GetDevices(ctx, token)
	if err != nil {
		log.Printf("[spotify] devices: get devices: %v", err)
		http.Error(w, "failed to get devices: "+err.Error(), 500)
		return
	}
	writeJSON(w, devs)
}

type transferReq struct {
	DeviceID string `json:"device_id"`
	Play     bool   `json:"play"`
}

func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	if !a.configured() {
		http.Error(w, "spotify not configured", 400)
		return
	}
	var req transferReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[spotify] transfer: decode: %v", err)
		http.Error(w, "bad json", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	token, err := a.ValidAccessToken(ctx)
	if err != nil {
		log.Printf("[spotify] transfer: not connected: %v", err)
		http.Error(w, "not connected: "+err.Error(), 400)
		return
	}
	if err := TransferPlayback(ctx, token, req.DeviceID, req.Play); err != nil {
		log.Printf("[spotify] transfer: transfer playback: %v", err)
		http.Error(w, "transfer failed: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
