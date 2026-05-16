// Package spotify implements PKCE OAuth, token storage, history sync,
// and Web Playback SDK token minting for Spotify integration.
package spotify

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Config struct {
	ClientID    string
	RedirectURI string
	FrontendURL string
}

type API struct {
	db   *sql.DB
	cfg  Config
	sync *Syncer
}

func New(db *sql.DB, cfg Config) *API {
	a := &API{db: db, cfg: cfg}
	a.sync = NewSyncer(db, cfg)
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
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (a *API) configured() bool {
	return a.cfg.ClientID != ""
}
