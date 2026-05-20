package apple

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Config carries runtime knobs supplied by main.go. Apple Music credentials
// themselves are stored in the DB (apple_music_config), not in env.
type Config struct {
	// AppName advertised to MusicKit (cosmetic, shown in the Apple ID auth popup).
	AppName string
}

type API struct {
	db             *sql.DB
	cfg            Config
	sync           *Syncer
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

func New(db *sql.DB, cfg Config) *API {
	if cfg.AppName == "" {
		cfg.AppName = "Lexicon"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &API{
		db:             db,
		cfg:            cfg,
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
	}
	a.sync = NewSyncer(db, a)
	return a
}

func (a *API) Shutdown() { a.shutdownCancel() }

// StartSyncer launches the background sync goroutine (idempotent-ish: the
// caller is expected to invoke this once at startup, like the Spotify one).
func (a *API) StartSyncer() { a.sync.Start(a.shutdownCtx) }

// Syncer exposes the syncer for code that needs to trigger ad-hoc runs.
func (a *API) Syncer() *Syncer { return a.sync }

func (a *API) Mount(r chi.Router) {
	r.Get("/api/apple/status", a.handleStatus)
	r.Post("/api/apple/config", a.handleSaveConfig)
	r.Delete("/api/apple/config", a.handleDeleteConfig)
	r.Get("/api/apple/musickit-config", a.handleMusicKitConfig)
	r.Post("/api/apple/connect", a.handleConnect)
	r.Post("/api/apple/disconnect", a.handleDisconnect)
	r.Post("/api/apple/sync", a.handleManualSync)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[apple] writeJSON: %v", err)
	}
}

// -----------------------------------------------------------------------
// /api/apple/status
// -----------------------------------------------------------------------

type statusResponse struct {
	Configured           bool   `json:"configured"`
	Connected            bool   `json:"connected"`
	TeamID               string `json:"team_id,omitempty"`
	KeyID                string `json:"key_id,omitempty"`
	Storefront           string `json:"storefront,omitempty"`
	DisplayName          string `json:"display_name,omitempty"`
	LastSyncedAt         int64  `json:"last_synced_at,omitempty"`
	DevTokenExpiresAt    int64  `json:"dev_token_expires_at,omitempty"`
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	var resp statusResponse

	var (
		teamID, keyID, storefront string
		devTokExp                 int64
	)
	err := a.db.QueryRowContext(r.Context(),
		`SELECT team_id, key_id, storefront, IFNULL(cached_dev_token_expires_at, 0)
		 FROM apple_music_config WHERE id=1`).Scan(&teamID, &keyID, &storefront, &devTokExp)
	switch {
	case err == nil:
		resp.Configured = true
		resp.TeamID = teamID
		resp.KeyID = keyID
		resp.Storefront = storefront
		resp.DevTokenExpiresAt = devTokExp
	case errors.Is(err, sql.ErrNoRows):
		// not configured
	default:
		log.Printf("[apple] status: read config: %v", err)
	}

	if resp.Configured {
		var (
			userStorefront, displayName string
			lastSyncedAt                int64
		)
		err := a.db.QueryRowContext(r.Context(),
			`SELECT storefront, IFNULL(display_name,''), last_synced_at
			 FROM apple_music_user WHERE id=1`).Scan(&userStorefront, &displayName, &lastSyncedAt)
		if err == nil {
			resp.Connected = true
			if userStorefront != "" {
				resp.Storefront = userStorefront
			}
			resp.DisplayName = displayName
			resp.LastSyncedAt = lastSyncedAt
		}
	}
	writeJSON(w, resp)
}

// -----------------------------------------------------------------------
// /api/apple/config — save / replace
// -----------------------------------------------------------------------

type saveConfigRequest struct {
	TeamID     string `json:"team_id"`
	KeyID      string `json:"key_id"`
	PrivateKey string `json:"private_key"`
	Storefront string `json:"storefront"`
}

func (a *API) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req saveConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), 400)
		return
	}
	req.TeamID = strings.TrimSpace(req.TeamID)
	req.KeyID = strings.TrimSpace(req.KeyID)
	req.PrivateKey = strings.TrimSpace(req.PrivateKey)
	req.Storefront = strings.ToLower(strings.TrimSpace(req.Storefront))
	if req.Storefront == "" {
		req.Storefront = "us"
	}

	// Validate by attempting a real sign. This catches bad .p8 contents
	// before we persist garbage to the DB.
	if err := ValidateConfig(req.TeamID, req.KeyID, req.PrivateKey); err != nil {
		http.Error(w, "invalid credentials: "+err.Error(), 400)
		return
	}

	now := time.Now().Unix()
	// Upsert single-row id=1
	_, err := a.db.ExecContext(r.Context(), `
		INSERT INTO apple_music_config(id, team_id, key_id, private_key, storefront, created_at, updated_at)
		VALUES(1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			team_id=excluded.team_id,
			key_id=excluded.key_id,
			private_key=excluded.private_key,
			storefront=excluded.storefront,
			cached_dev_token=NULL,
			cached_dev_token_expires_at=0,
			updated_at=excluded.updated_at
	`, req.TeamID, req.KeyID, req.PrivateKey, req.Storefront, now, now)
	if err != nil {
		log.Printf("[apple] save config: %v", err)
		http.Error(w, "db write failed", 500)
		return
	}

	// Mint a real token to populate the cache (and surface any Apple-side
	// rejection synchronously — though /v1/test probes are not currently
	// enabled here because they require a real network call we don't want
	// to delay the save by; clients call /api/apple/musickit-config next
	// which exercises the same path).
	tok, err := MintDeveloperToken(r.Context(), a.db)
	if err != nil {
		log.Printf("[apple] post-save mint: %v", err)
		http.Error(w, "saved, but could not mint token: "+err.Error(), 500)
		return
	}

	writeJSON(w, map[string]any{
		"ok":              true,
		"developer_token": tok,
	})
}

func (a *API) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM apple_music_user WHERE id=1`); err != nil {
		log.Printf("[apple] delete user: %v", err)
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM apple_music_config WHERE id=1`); err != nil {
		log.Printf("[apple] delete config: %v", err)
		http.Error(w, "delete failed", 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// -----------------------------------------------------------------------
// /api/apple/musickit-config — what the browser needs to init MusicKit JS
// -----------------------------------------------------------------------

type musicKitConfigResponse struct {
	DeveloperToken string `json:"developer_token"`
	AppName        string `json:"app_name"`
	Storefront     string `json:"storefront"`
}

func (a *API) handleMusicKitConfig(w http.ResponseWriter, r *http.Request) {
	tok, err := MintDeveloperToken(r.Context(), a.db)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			http.Error(w, "apple music not configured", 400)
			return
		}
		log.Printf("[apple] musickit-config: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}
	var storefront string
	if err := a.db.QueryRowContext(r.Context(),
		`SELECT storefront FROM apple_music_config WHERE id=1`).Scan(&storefront); err != nil {
		storefront = "us"
	}
	writeJSON(w, musicKitConfigResponse{
		DeveloperToken: tok,
		AppName:        a.cfg.AppName,
		Storefront:     storefront,
	})
}

// -----------------------------------------------------------------------
// /api/apple/connect — store MUT supplied by MusicKit JS
// -----------------------------------------------------------------------

type connectRequest struct {
	MusicUserToken string `json:"music_user_token"`
}

func (a *API) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), 400)
		return
	}
	req.MusicUserToken = strings.TrimSpace(req.MusicUserToken)
	if req.MusicUserToken == "" {
		http.Error(w, "music_user_token required", 400)
		return
	}

	devTok, err := MintDeveloperToken(r.Context(), a.db)
	if err != nil {
		http.Error(w, "developer token: "+err.Error(), 500)
		return
	}

	// Confirm the MUT works by fetching the storefront. This also tells us
	// the user's actual storefront, which may differ from what they typed.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	storefront, err := FetchUserStorefront(ctx, devTok, req.MusicUserToken)
	if err != nil {
		log.Printf("[apple] connect: storefront probe failed: %v", err)
		http.Error(w, "apple rejected the user token: "+err.Error(), 400)
		return
	}

	now := time.Now().Unix()
	_, err = a.db.ExecContext(r.Context(), `
		INSERT INTO apple_music_user(id, music_user_token, storefront, display_name, last_synced_at, connected_at)
		VALUES(1, ?, ?, '', 0, ?)
		ON CONFLICT(id) DO UPDATE SET
			music_user_token=excluded.music_user_token,
			storefront=excluded.storefront,
			last_synced_at=0,
			connected_at=excluded.connected_at
	`, req.MusicUserToken, storefront, now)
	if err != nil {
		log.Printf("[apple] connect: store mut: %v", err)
		http.Error(w, "db write failed", 500)
		return
	}

	// Kick off an immediate sync so the user sees data within ~30s.
	go func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer bgCancel()
		if err := a.sync.RunOnce(bgCtx); err != nil {
			log.Printf("[apple] post-connect sync: %v", err)
		}
	}()

	writeJSON(w, map[string]any{"ok": true, "storefront": storefront})
}

func (a *API) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM apple_music_user WHERE id=1`); err != nil {
		log.Printf("[apple] disconnect: %v", err)
		http.Error(w, "delete failed", 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// -----------------------------------------------------------------------
// /api/apple/sync — manual trigger
// -----------------------------------------------------------------------

func (a *API) handleManualSync(w http.ResponseWriter, r *http.Request) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := a.sync.RunOnce(ctx); err != nil {
			log.Printf("[apple] manual sync: %v", err)
		}
	}()
	writeJSON(w, map[string]bool{"started": true})
}

// -----------------------------------------------------------------------
// Helpers exposed to the recommender
// -----------------------------------------------------------------------

// CurrentTokens returns a developer token and the user token (if connected).
// Returns ErrNotConfigured if no creds saved; returns ("", "", nil) with the
// connected=false signal if creds exist but the user hasn't authorized yet.
func (a *API) CurrentTokens(ctx context.Context) (devTok, mut string, err error) {
	devTok, err = MintDeveloperToken(ctx, a.db)
	if err != nil {
		return "", "", err
	}
	err = a.db.QueryRowContext(ctx, `SELECT music_user_token FROM apple_music_user WHERE id=1`).Scan(&mut)
	if errors.Is(err, sql.ErrNoRows) {
		return devTok, "", nil
	}
	return devTok, mut, err
}
