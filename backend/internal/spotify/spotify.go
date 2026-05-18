// Package spotify implements PKCE OAuth, token storage, history sync,
// and Web Playback SDK token minting for Spotify integration.
package spotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type verifierEntry struct {
	Verifier  string
	CreatedAt time.Time
}

type Config struct {
	ClientID    string
	RedirectURI string
	FrontendURL string
}

type API struct {
	db        *sql.DB
	cfg       Config
	sync      *Syncer
	verifiers sync.Map
}

func New(db *sql.DB, cfg Config) *API {
	a := &API{db: db, cfg: cfg}
	a.sync = NewSyncer(db, cfg)
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			cutoff := time.Now().Add(-10 * time.Minute)
			a.verifiers.Range(func(key, value any) bool {
				if entry, ok := value.(verifierEntry); ok && entry.CreatedAt.Before(cutoff) {
					a.verifiers.Delete(key)
				}
				return true
			})
		}
	}()
	return a
}

// Syncer returns the background syncer so main can start it.
func (a *API) Syncer() *Syncer { return a.sync }

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
	json.NewEncoder(w).Encode(v)
}

func (a *API) configured() bool {
	return a.cfg.ClientID != ""
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
		http.Error(w, "not connected: "+err.Error(), 400)
		return
	}
	devs, err := GetDevices(ctx, token)
	if err != nil {
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
		http.Error(w, "bad json", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	token, err := a.ValidAccessToken(ctx)
	if err != nil {
		http.Error(w, "not connected: "+err.Error(), 400)
		return
	}
	if err := TransferPlayback(ctx, token, req.DeviceID, req.Play); err != nil {
		http.Error(w, "transfer failed: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
