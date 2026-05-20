package recommender

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/apple"
	"github.com/kevin/lexicon/internal/spotify"
	"github.com/kevin/lexicon/internal/websearch"
)

// httpClient is the shared HTTP client for DeepSeek API requests.
// It uses a 30-second timeout to prevent indefinite hangs.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

type DeepSeekConfig struct {
	APIKey   string
	Model    string
	Thinking string
	BaseURL  string
}

type API struct {
	db      *sql.DB
	cfg     DeepSeekConfig
	ws      *websearch.WebSearch
	spotify *spotify.API
	apple   *apple.API
}

func New(db *sql.DB, cfg DeepSeekConfig, ws *websearch.WebSearch, spotifyAPI *spotify.API, appleAPI *apple.API) *API {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	return &API{db: db, cfg: cfg, ws: ws, spotify: spotifyAPI, apple: appleAPI}
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/recommendations", a.get)
	r.Post("/api/recommendations/refresh", a.refresh)
	r.Post("/api/recommendations/chat", a.chat)
	r.Post("/api/recommendations/playlist", a.playlist)
}

type Item struct {
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Reason  string `json:"reason"`
	TrackID *int64 `json:"track_id,omitempty"`
	Type    string `json:"type"` // "library" | "discover"
}

type RecsPayload struct {
	Summary string `json:"summary"`
	Trends  string `json:"trends"`
	Items   []Item `json:"items"`
}

type PlaylistTrack struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Reason string `json:"reason"`
}

type PlaylistPayload struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tracks      []PlaylistTrack  `json:"tracks"`
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	var payload string
	var createdAt int64
	filterType := r.URL.Query().Get("type")
	var query string
	var args []interface{}
	if filterType != "" {
		query = `SELECT payload, created_at FROM recommendations WHERE type = ? ORDER BY id DESC LIMIT 1`
		args = append(args, filterType)
	} else {
		query = `SELECT payload, created_at FROM recommendations WHERE (type IS NULL OR type = '') ORDER BY id DESC LIMIT 1`
	}
	err := a.db.QueryRowContext(r.Context(), query, args...).Scan(&payload, &createdAt)
	if err == sql.ErrNoRows {
		writeJSON(w, map[string]interface{}{"empty": true})
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"created_at":%d,"data":%s}`, createdAt, payload)
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	if a.cfg.APIKey == "" {
		http.Error(w, "DEEPSEEK_API_KEY not configured", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	profile, err := a.buildProfile(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Enrich profile with Spotify top data if connected
	if a.spotify != nil {
		spCtx, spCancel := context.WithTimeout(ctx, 45*time.Second)
		spotifyProfile := a.buildSpotifyProfile(spCtx)
		spCancel()
		if spotifyProfile != "" {
			profile = profile + "\n" + spotifyProfile
		}
	}

	// Enrich profile with Apple Music data if connected
	if a.apple != nil {
		apCtx, apCancel := context.WithTimeout(ctx, 45*time.Second)
		appleProfile := a.buildAppleProfile(apCtx)
		apCancel()
		if appleProfile != "" {
			profile = profile + "\n" + appleProfile
		}
	}

	payload, err := a.callDeepSeek(ctx, profile)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	raw, _ := json.Marshal(payload)
	_, err = a.db.ExecContext(ctx, `INSERT INTO recommendations(payload) VALUES(?)`, string(raw))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, payload)
}

func (a *API) playlist(w http.ResponseWriter, r *http.Request) {
	if a.cfg.APIKey == "" {
		http.Error(w, "DEEPSEEK_API_KEY not configured", 400)
		return
	}

	force := r.URL.Query().Get("force") == "true"

	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 25
	}
	if count > 100 {
		count = 100
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	profile, err := a.buildProfile(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Enrich profile with Spotify top data if connected
	if a.spotify != nil {
		spCtx, spCancel := context.WithTimeout(ctx, 45*time.Second)
		spotifyProfile := a.buildSpotifyProfile(spCtx)
		spCancel()
		if spotifyProfile != "" {
			profile = profile + "\n" + spotifyProfile
		}
	}

	// Enrich profile with Apple Music data if connected
	if a.apple != nil {
		apCtx, apCancel := context.WithTimeout(ctx, 45*time.Second)
		appleProfile := a.buildAppleProfile(apCtx)
		apCancel()
		if appleProfile != "" {
			profile = profile + "\n" + appleProfile
		}
	}

	profileHash := hashProfile(profile)

	// Cache lookup: check for a playlist generated from the same profile within the last hour
	if !force {
		var cachedPayload string
		err := a.db.QueryRowContext(ctx,
			`SELECT payload FROM recommendations
			 WHERE type='playlist' AND prompt_hash=?
			 AND created_at > strftime('%s','now','-1 hour')
			 ORDER BY id DESC LIMIT 1`, profileHash).Scan(&cachedPayload)
		if err == nil {
			var out PlaylistPayload
			if json.Unmarshal([]byte(cachedPayload), &out) == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			if err := json.NewEncoder(w).Encode(out); err != nil {
				log.Printf("[recommender] playlist cache encode: %v", err)
			}
				return
			}
		}
	}

	// Web search enrichment: try to find latest album tracks from top artist
	searchContext := ""
	if a.ws != nil {
		if topArtist := a.extractTopArtist(profile); topArtist != "" {
			tracks, err := a.ws.ResolveLatestAlbumTracks(ctx, topArtist)
			if err != nil {
				log.Printf("[websearch] latest album error: %v", err)
			}
			if len(tracks) > 0 {
				var sb strings.Builder
				sb.WriteString("\n--- WEB SEARCH RESULTS ---\n")
				sb.WriteString(fmt.Sprintf("Latest album by %s: %s\nTracks found online:\n", topArtist, tracks[0].Album))
				for i, t := range tracks {
					sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, t.Title))
				}
				sb.WriteString("--- END SEARCH RESULTS ---\n")
				searchContext = sb.String()
			}
		}
	}

	prompt := fmt.Sprintf(`Given this user's listening profile, generate a cohesive playlist.
Return ONLY valid JSON with this shape:
{
  "name": "catchy playlist name",
  "description": "1-2 sentence vibe description",
  "tracks": [
    {"title":"...", "artist":"...", "reason":"brief why this fits"}
  ]
}

PROFILE LEGEND:
- "Top artists/tracks (90d)" = what they've been playing from their local library
- "Spotify Top Artists/Tracks (last 6 months)" = what they stream on Spotify — use this to understand their broader taste beyond local files
- "User's Spotify Playlists" = playlists they've created — great source of thematic inspiration
- "User's Spotify Library" = songs they've saved/liked on Spotify
- "Followed Artists" = artists they actively follow on Spotify
- "Apple Music Top Artists" = synthesized from heavy rotation + recently played + library
- "Apple Music Heavy Rotation" / "Recently Played" = what they listen to on Apple Music
- "Apple Music Library Playlists" = their own Apple Music playlists
- "Apple Music Personalized Recommendation Buckets" = Apple's algorithmic mixes for this user (great inspiration)
- "Recently played" = most recent local plays

Rules:
- Generate exactly %d tracks
- Name should be creative and thematic (not generic like "My Playlist")
- Mix of songs the user likely has and new discoveries
- Be specific and personal — reference artists and patterns from the profile in your reasons
- If Spotify or Apple Music data shows different taste than local library, weight the streaming data more heavily (it reflects broader listening). Cross-reference both platforms when present.
- Output ONLY valid JSON, no prose, no code fences.
%s
PROFILE:
%s`, count, searchContext, profile)

	reply, err := a.deepseekChat(ctx, []map[string]string{
		{"role": "system", "content": "You are a music curator. Respond with JSON only."},
		{"role": "user", "content": prompt},
	}, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	var out PlaylistPayload
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		http.Error(w, "invalid playlist JSON: "+err.Error(), 500)
		return
	}

	// Store result in recommendations table for future cache hits
	raw, _ := json.Marshal(out)
	_, _ = a.db.ExecContext(ctx,
		`INSERT INTO recommendations(type, prompt_hash, payload) VALUES('playlist', ?, ?)`,
		profileHash, string(raw))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Printf("[recommender] playlist encode: %v", err)
	}
}

type chatReq struct {
	Message string `json:"message"`
}

func (a *API) chat(w http.ResponseWriter, r *http.Request) {
	if a.cfg.APIKey == "" {
		http.Error(w, "DEEPSEEK_API_KEY not configured", 400)
		return
	}
	var req chatReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	// 120s timeout: buildProfile + buildSpotifyProfile (5 API calls) + DeepSeek call
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	profile, _ := a.buildProfile(ctx)

	// Enrich profile with Spotify top data if connected
	if a.spotify != nil {
		// Use a dedicated sub-context for Spotify to avoid starving the LLM call
		spCtx, spCancel := context.WithTimeout(ctx, 45*time.Second)
		spotifyProfile := a.buildSpotifyProfile(spCtx)
		spCancel()
		if spotifyProfile != "" {
			profile = profile + "\n" + spotifyProfile
		}
	}

	// Enrich profile with Apple Music data if connected
	if a.apple != nil {
		apCtx, apCancel := context.WithTimeout(ctx, 45*time.Second)
		appleProfile := a.buildAppleProfile(apCtx)
		apCancel()
		if appleProfile != "" {
			profile = profile + "\n" + appleProfile
		}
	}

	// Extract count from user message (e.g., "30-track", "50 songs")
	count := extractCountFromMessage(req.Message)
	if count <= 0 {
		count = 25
	}
	if count > 100 {
		count = 100
	}

	// Web search enrichment for album/artist queries
	searchContext := ""
	if q := websearch.DetectSearchQuery(req.Message); q != "" && a.ws != nil {
		tracks, err := a.ws.ResolveAlbumTracks(ctx, q, "")
		if err != nil {
			log.Printf("[websearch] album resolve error: %v", err)
		}
		if len(tracks) == 0 {
			// Try latest album by inferred artist
			tracks, _ = a.ws.ResolveLatestAlbumTracks(ctx, q)
		}
		if len(tracks) > 0 {
			var sb strings.Builder
			sb.WriteString("\n--- WEB SEARCH RESULTS ---\n")
			sb.WriteString(fmt.Sprintf("Album: %s by %s\nTracks found online:\n", tracks[0].Album, tracks[0].Artist))
			for i, t := range tracks {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, t.Title))
			}
			sb.WriteString("--- END SEARCH RESULTS ---\n")
			searchContext = sb.String()
		}
	}

	system := fmt.Sprintf(`You are a music curator with deep knowledge of the user's taste. ALWAYS respond with ONLY a single valid JSON object. No markdown, no prose outside the JSON, no code fences.

If the user asks for a playlist, use this shape:
{"message":"A short conversational reply","playlist":{"name":"Creative Playlist Name","description":"1-2 sentence vibe","tracks":[{"title":"...","artist":"...","reason":"..."}]}}

If the user asks a normal question (not about making a playlist), use this shape:
{"message":"Your concise answer here."}

PROFILE LEGEND:
- "Top artists/tracks (90d)" = what they've been playing from their local library
- "Spotify Top Artists/Tracks (last 6 months)" = what they stream on Spotify — use this to understand their broader taste beyond local files
- "User's Spotify Playlists" = playlists they've created — great source of thematic inspiration
- "User's Spotify Library" = songs they've saved/liked on Spotify
- "Followed Artists" = artists they actively follow on Spotify
- "Apple Music Top Artists" = synthesized from heavy rotation + recently played + library
- "Apple Music Heavy Rotation" / "Recently Played" = what they listen to on Apple Music
- "Apple Music Library Playlists" = their own Apple Music playlists
- "Apple Music Personalized Recommendation Buckets" = Apple's algorithmic mixes for this user
- "Recently played" = most recent local plays

Rules:
- For playlist requests: generate exactly %d tracks, creative thematic name, reference specific artists and patterns from the profile in your reasons.
- For normal questions: answer concisely in the "message" field, referencing the user's taste profile when relevant.
- ALWAYS include the "message" field.
- NEVER output text outside the JSON object.
- If Spotify or Apple Music data shows different taste than local library, weight the streaming data more heavily. When both platforms are present, look for overlaps (shared artists) and divergences.
%s
---
USER LISTENING PROFILE:
---
REMEMBER: Always return a JSON object with a "message" field.
%s`, count, searchContext, profile)
	reply, err := a.deepseekChatWithRetry(ctx, []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": req.Message},
	}, &DSRespFmt{Type: "json_object"}, func(raw string) error {
		var dummy struct {
			Message  string          `json:"message"`
			Playlist PlaylistPayload `json:"playlist"`
		}
		return json.Unmarshal([]byte(raw), &dummy)
	})
	if err != nil {
		log.Printf("deepseek chat error: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	var parsed struct {
		Message  string          `json:"message"`
		Playlist PlaylistPayload `json:"playlist"`
	}
	if err := json.Unmarshal([]byte(reply), &parsed); err != nil {
		// Fallback: if the model returned plain text instead of JSON, wrap it
		log.Printf("chat JSON parse failed (fallback to text): %v | raw reply: %q", err, reply)
		writeJSON(w, map[string]string{"reply": strings.TrimSpace(reply)})
		return
	}

	if len(parsed.Playlist.Tracks) > 0 {
		writeJSON(w, map[string]interface{}{
			"reply":    parsed.Message,
			"playlist": parsed.Playlist,
		})
		return
	}

	writeJSON(w, map[string]string{"reply": parsed.Message})
}

// buildProfile renders a compact natural-language summary of recent listening.
func (a *API) buildProfile(ctx context.Context) (string, error) {
	var b strings.Builder

	// Top artists last 90d
	rows, err := a.db.QueryContext(ctx, `
		SELECT IFNULL(COALESCE(NULLIF(t.album_artist,''),t.artist),''), COUNT(*)
		FROM plays p JOIN tracks t ON t.id=p.track_id
		WHERE p.started_at > strftime('%s','now','-90 days')
		GROUP BY 1 ORDER BY 2 DESC LIMIT 10`)
	if err == nil {
		fmt.Fprintln(&b, "Top artists (90d):")
		for rows.Next() {
			var a string
			var n int
			if err := rows.Scan(&a, &n); err != nil {
				log.Printf("[recommender] profile top-artists scan: %v", err)
				continue
			}
			if a == "" {
				continue
			}
			fmt.Fprintf(&b, "  - %s (%d plays)\n", a, n)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[recommender] profile top-artists query error: %v", err)
		}
		rows.Close()
	}

	// Top genres
	rows, err = a.db.QueryContext(ctx, `
		SELECT IFNULL(t.genre,''), COUNT(*) FROM plays p JOIN tracks t ON t.id=p.track_id
		WHERE p.started_at > strftime('%s','now','-90 days')
		GROUP BY 1 HAVING IFNULL(t.genre,'')!='' ORDER BY 2 DESC LIMIT 8`)
	if err == nil {
		fmt.Fprintln(&b, "Top genres (90d):")
		for rows.Next() {
			var g string
			var n int
			rows.Scan(&g, &n)
			fmt.Fprintf(&b, "  - %s (%d)\n", g, n)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[recommender] profile top-genres query error: %v", err)
		}
		rows.Close()
	}

	// Recent
	rows, err = a.db.QueryContext(ctx, `
		SELECT t.title, IFNULL(t.artist,''), t.media_kind FROM plays p JOIN tracks t ON t.id=p.track_id
		ORDER BY p.started_at DESC LIMIT 15`)
	if err == nil {
		fmt.Fprintln(&b, "Recently played:")
		for rows.Next() {
			var t, ar, k string
			rows.Scan(&t, &ar, &k)
			fmt.Fprintf(&b, "  - [%s] %s — %s\n", k, t, ar)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[recommender] profile recent-played query error: %v", err)
		}
		rows.Close()
	}

	// Library snapshot
	var tCount, aCount int
	a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tracks WHERE media_kind='music'`).Scan(&tCount)
	a.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT COALESCE(NULLIF(album_artist,''),artist)) FROM tracks WHERE media_kind='music'`).Scan(&aCount)
	fmt.Fprintf(&b, "Library: %d tracks, %d artists.\n", tCount, aCount)

	return b.String(), nil
}

// buildSpotifyProfile fetches the user's Spotify data to enrich the LLM profile.
// Includes: top artists, top tracks, playlists, saved tracks, and followed artists.
// Each API call has its own 15s timeout to avoid one slow call blocking others.
func (a *API) buildSpotifyProfile(ctx context.Context) string {
	if a.spotify == nil {
		return ""
	}

	// Get a valid access token
	accessToken, err := a.spotify.ValidAccessToken(ctx)
	if err != nil {
		log.Printf("[recommender] spotify token: %v", err)
		return ""
	}

	var b strings.Builder

	// Helper: run a Spotify API call with a per-call timeout
	call := func(fn func(context.Context) (string, error)) {
		callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := fn(callCtx)
		if err != nil {
			log.Printf("[recommender] spotify: %v", err)
			return
		}
		if result != "" {
			fmt.Fprint(&b, result)
		}
	}

	// 1. Fetch top artists from Spotify
	call(func(callCtx context.Context) (string, error) {
		topArtists, err := spotify.FetchTopArtists(callCtx, accessToken, 20)
		if err != nil {
			return "", fmt.Errorf("top artists: %w", err)
		}
		if len(topArtists.Items) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Spotify Top Artists (last 6 months):")
		for i, artist := range topArtists.Items {
			if i >= 10 {
				break
			}
			genreStr := ""
			if len(artist.Genres) > 0 {
				genreStr = fmt.Sprintf(" [%s]", strings.Join(artist.Genres[:min(3, len(artist.Genres))], ", "))
			}
			fmt.Fprintf(&sb, "  - %s%s\n", artist.Name, genreStr)
		}
		return sb.String(), nil
	})

	// 2. Fetch top tracks from Spotify
	call(func(callCtx context.Context) (string, error) {
		topTracks, err := spotify.FetchTopTracks(callCtx, accessToken, 20)
		if err != nil {
			return "", fmt.Errorf("top tracks: %w", err)
		}
		if len(topTracks.Items) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Spotify Top Tracks (last 6 months):")
		for i, track := range topTracks.Items {
			if i >= 10 {
				break
			}
			artists := make([]string, 0, len(track.Artists))
			for _, a := range track.Artists {
				artists = append(artists, a.Name)
			}
			fmt.Fprintf(&sb, "  - %s — %s\n", track.Name, strings.Join(artists, ", "))
		}
		return sb.String(), nil
	})

	// 3. Fetch user's playlists (names, descriptions, track counts)
	call(func(callCtx context.Context) (string, error) {
		playlists, err := spotify.FetchUserPlaylists(callCtx, accessToken, 50)
		if err != nil {
			return "", fmt.Errorf("playlists: %w", err)
		}
		if len(playlists) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "User's Spotify Playlists:")
		for i, pl := range playlists {
			if i >= 15 {
				break
			}
			visibility := ""
			if pl.Collaborative {
				visibility = " (collaborative)"
			} else if !pl.Public {
				visibility = " (private)"
			}
			desc := ""
			if pl.Description != "" {
				desc = fmt.Sprintf(" — %s", truncate(pl.Description, 60))
			}
			fmt.Fprintf(&sb, "  - %s%s [%d tracks]%s\n", pl.Name, visibility, pl.Tracks.Total, desc)
		}
		return sb.String(), nil
	})

	// 4. Fetch saved/liked tracks (sample of 50 most recent)
	call(func(callCtx context.Context) (string, error) {
		savedTracks, err := spotify.FetchSavedTracks(callCtx, accessToken, 50)
		if err != nil {
			return "", fmt.Errorf("saved tracks: %w", err)
		}
		if len(savedTracks) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "User's Spotify Library: %d saved tracks (showing recent %d)\n", len(savedTracks), min(10, len(savedTracks)))
		for i, st := range savedTracks {
			if i >= 10 {
				break
			}
			artists := make([]string, 0, len(st.Track.Artists))
			for _, a := range st.Track.Artists {
				artists = append(artists, a.Name)
			}
			fmt.Fprintf(&sb, "  - %s — %s\n", st.Track.Name, strings.Join(artists, ", "))
		}
		return sb.String(), nil
	})

	// 5. Fetch followed artists
	call(func(callCtx context.Context) (string, error) {
		followedArtists, err := spotify.FetchFollowedArtists(callCtx, accessToken, 50)
		if err != nil {
			return "", fmt.Errorf("followed artists: %w", err)
		}
		if len(followedArtists) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Followed Artists on Spotify (%d total):\n", len(followedArtists))
		for i, artist := range followedArtists {
			if i >= 10 {
				break
			}
			genreStr := ""
			if len(artist.Genres) > 0 {
				genreStr = fmt.Sprintf(" [%s]", strings.Join(artist.Genres[:min(2, len(artist.Genres))], ", "))
			}
			fmt.Fprintf(&sb, "  - %s%s\n", artist.Name, genreStr)
		}
		return sb.String(), nil
	})

	return b.String()
}

// buildAppleProfile fetches the user's Apple Music data to enrich the LLM
// profile. Mirrors buildSpotifyProfile but adapted to Apple's data model:
// Apple has no first-class "top artists last 6mo" endpoint, so we synthesize
// one from heavy rotation + recently played + library artists.
//
// Each section is bounded to ~10 lines to stay within the prompt budget.
func (a *API) buildAppleProfile(ctx context.Context) string {
	if a.apple == nil {
		return ""
	}
	devTok, mut, err := a.apple.CurrentTokens(ctx)
	if err != nil || mut == "" {
		// Not configured or not connected — silent no-op.
		return ""
	}

	var b strings.Builder

	call := func(fn func(context.Context) (string, error)) {
		callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := fn(callCtx)
		if err != nil {
			log.Printf("[recommender] apple: %v", err)
			return
		}
		if result != "" {
			fmt.Fprint(&b, result)
		}
	}

	// 1. Synthesized top artists (heavy rotation + recently played + library).
	call(func(callCtx context.Context) (string, error) {
		artists, err := apple.SynthesizeTopArtists(callCtx, devTok, mut, 10)
		if err != nil {
			return "", fmt.Errorf("synth top artists: %w", err)
		}
		if len(artists) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Apple Music Top Artists (heavy rotation + recent + library):")
		for _, ta := range artists {
			genres := ""
			if len(ta.Genres) > 0 {
				genres = fmt.Sprintf(" [%s]", strings.Join(ta.Genres[:min(3, len(ta.Genres))], ", "))
			}
			fmt.Fprintf(&sb, "  - %s%s\n", ta.Name, genres)
		}
		return sb.String(), nil
	})

	// 2. Heavy rotation tracks (raw).
	call(func(callCtx context.Context) (string, error) {
		hr, err := apple.FetchHeavyRotation(callCtx, devTok, mut, 10)
		if err != nil {
			return "", fmt.Errorf("heavy rotation: %w", err)
		}
		if len(hr.Data) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Apple Music Heavy Rotation:")
		for i, s := range hr.Data {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&sb, "  - %s — %s\n", s.Attributes.Name, s.Attributes.ArtistName)
		}
		return sb.String(), nil
	})

	// 3. Recently played.
	call(func(callCtx context.Context) (string, error) {
		rp, err := apple.FetchRecentlyPlayed(callCtx, devTok, mut, 30)
		if err != nil {
			return "", fmt.Errorf("recently played: %w", err)
		}
		if len(rp.Data) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Apple Music Recently Played (last 10):")
		for i, s := range rp.Data {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&sb, "  - %s — %s\n", s.Attributes.Name, s.Attributes.ArtistName)
		}
		return sb.String(), nil
	})

	// 4. Library playlists.
	call(func(callCtx context.Context) (string, error) {
		pl, err := apple.FetchLibraryPlaylists(callCtx, devTok, mut, 30)
		if err != nil {
			return "", fmt.Errorf("library playlists: %w", err)
		}
		if len(pl.Data) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Apple Music Library Playlists:")
		for i, p := range pl.Data {
			if i >= 10 {
				break
			}
			desc := ""
			if p.Attributes.Description.Standard != "" {
				desc = fmt.Sprintf(" — %s", truncate(p.Attributes.Description.Standard, 60))
			}
			fmt.Fprintf(&sb, "  - %s%s\n", p.Attributes.Name, desc)
		}
		return sb.String(), nil
	})

	// 5. Apple's own personalized recommendations (editorial / algorithmic mixes).
	call(func(callCtx context.Context) (string, error) {
		recs, err := apple.FetchRecommendations(callCtx, devTok, mut, 10)
		if err != nil {
			return "", fmt.Errorf("recommendations: %w", err)
		}
		if len(recs.Data) == 0 {
			return "", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "Apple Music Personalized Recommendation Buckets:")
		for i, r := range recs.Data {
			if i >= 10 {
				break
			}
			title := r.Attributes.Title.StringForDisplay
			if title == "" {
				continue
			}
			reason := r.Attributes.Reason.StringForDisplay
			if reason != "" {
				fmt.Fprintf(&sb, "  - %s — %s\n", title, truncate(reason, 80))
			} else {
				fmt.Fprintf(&sb, "  - %s\n", title)
			}
		}
		return sb.String(), nil
	})

	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *API) callDeepSeek(ctx context.Context, profile string) (RecsPayload, error) {
	prompt := `You are a music & podcast recommendation engine. Based on the user's listening profile below, recommend 8 songs and 4 podcasts they might enjoy.

PROFILE LEGEND:
- "Top artists/tracks (90d)" = what they've been playing from their local library
- "Spotify Top Artists/Tracks (last 6 months)" = what they stream on Spotify — broader taste beyond local files
- "User's Spotify Playlists" = playlists they've created
- "User's Spotify Library" = songs they've saved/liked on Spotify
- "Followed Artists" = artists they actively follow on Spotify
- "Apple Music Top Artists" = synthesized from heavy rotation + recently played + library
- "Apple Music Heavy Rotation" / "Recently Played" = what they listen to on Apple Music
- "Apple Music Library Playlists" = their own Apple Music playlists
- "Apple Music Personalized Recommendation Buckets" = Apple's algorithmic mixes for this user
- "Recently played" = most recent local plays

` + profile + `

Respond with JSON in this exact format:
{
  "items": [
    {"title": "...", "artist": "...", "album": "...", "type": "library|discover", "genre": "pop|rock|...", "reason": "..."}
  ],
  "summary": "1-2 sentence overview"
}

Rules:
- "library" = user has this in their collection
- "discover" = new suggestion they don't own
- Be specific — reference artists and patterns from the profile in your reasons
- If Spotify or Apple Music data shows different taste than local library, weight the streaming data more heavily. When both platforms are present, cross-reference for overlaps and divergences.
- Output ONLY valid JSON, no prose, no code fences.`

	reply, err := a.deepseekChatWithRetry(ctx, []map[string]string{
		{"role": "system", "content": "You are a music & podcast recommendation engine. Respond with JSON only."},
		{"role": "user", "content": prompt},
	}, nil, func(raw string) error {
		var dummy RecsPayload
		return json.Unmarshal([]byte(raw), &dummy)
	})
	if err != nil {
		return RecsPayload{}, err
	}

	var out RecsPayload
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		log.Printf("[recommender] deepseek parse: %v", err)
		return RecsPayload{}, err
	}

	// Resolve track_id for all items; upgrade "discover" items that exist in library
	for i, it := range out.Items {
		var id int64
		err := a.db.QueryRowContext(ctx,
			`SELECT id FROM tracks WHERE LOWER(title)=LOWER(?) AND LOWER(IFNULL(artist,''))=LOWER(?) LIMIT 1`,
			it.Title, it.Artist).Scan(&id)
		if err == nil {
			out.Items[i].TrackID = &id
			if it.Type == "discover" {
				out.Items[i].Type = "library"
			}
		}
	}
	return out, nil
}

// DSMessage represents a chat message for the DeepSeek API.
type DSMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DSRequest represents a chat completion request for the DeepSeek API.
type DSRequest struct {
	Model           string       `json:"model"`
	Messages        []DSMessage  `json:"messages"`
	Temperature     float64      `json:"temperature"`
	ReasoningEffort string       `json:"reasoning_effort,omitempty"`
	ResponseFormat  *DSRespFmt   `json:"response_format,omitempty"`
}

// DSRespFmt specifies the response format for DeepSeek API calls.
type DSRespFmt struct {
	Type string `json:"type"`
}

// DSResponse represents a chat completion response from the DeepSeek API.
type DSResponse struct {
	Choices []struct {
		Message DSMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (a *API) deepseekChatWithRetry(
	ctx context.Context,
	msgs []map[string]string,
	respFmt *DSRespFmt,
	parseFn func(string) error,
) (string, error) {
	reply, err := a.deepseekChat(ctx, msgs, respFmt)
	if err != nil {
		return "", err
	}

	// Clean up common artifacts
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	// Try to parse
	if err := parseFn(reply); err == nil {
		return reply, nil
	}

	// --- Retry once with stricter instructions ---
	log.Printf("recommender: first parse failed, retrying with stricter prompt. err=%v reply=%q", err, truncate(reply, 200))

	retryMsgs := make([]map[string]string, len(msgs)+1)
	copy(retryMsgs, msgs)
	retryMsgs[len(msgs)] = map[string]string{
		"role":    "user",
		"content": "Your previous response was not valid JSON and could not be parsed. You MUST respond with ONLY a valid JSON object — no markdown, no code fences, no prose outside the JSON. Return ONLY the JSON object.",
	}

	reply, err = a.deepseekChat(ctx, retryMsgs, respFmt)
	if err != nil {
		return "", fmt.Errorf("retry failed: %w", err)
	}

	// Clean up again
	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	if err := parseFn(reply); err != nil {
		return "", fmt.Errorf("invalid JSON after retry: %w | raw: %s", err, truncate(reply, 200))
	}

	return reply, nil
}

func (a *API) deepseekChat(ctx context.Context, msgs []map[string]string, respFmt *DSRespFmt) (string, error) {
	dmsgs := make([]DSMessage, len(msgs))
	for i, m := range msgs {
		dmsgs[i] = DSMessage{Role: m["role"], Content: m["content"]}
	}
	reqBody := DSRequest{
		Model:          a.cfg.Model,
		Messages:       dmsgs,
		Temperature:    0.7,
		ResponseFormat: respFmt,
	}
	// reasoning_effort is only valid for deepseek-reasoner models
	if strings.Contains(a.cfg.Model, "reasoner") && a.cfg.Thinking != "" {
		reqBody.ReasoningEffort = a.cfg.Thinking
	}
	buf, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("deepseek %d: %s", resp.StatusCode, string(body))
	}
	var dr DSResponse
	if err := json.Unmarshal(body, &dr); err != nil {
		return "", fmt.Errorf("decode: %v", err)
	}
	if dr.Error != nil {
		return "", fmt.Errorf("deepseek error: %s", dr.Error.Message)
	}
	if len(dr.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}
	content := strings.TrimSpace(dr.Choices[0].Message.Content)
	if content == "" {
		log.Printf("deepseek returned empty content | model=%s status=%d body=%s", a.cfg.Model, resp.StatusCode, string(body))
		return "", fmt.Errorf("empty response from model")
	}
	return content, nil
}

// extractTopArtist parses the first artist from the "Top artists (90d):" section
// of the profile text, returning "" if not found.
func (a *API) extractTopArtist(profile string) string {
	lines := strings.Split(profile, "\n")
	inTop := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Top artists (90d):") {
			inTop = true
			continue
		}
		if inTop && strings.HasPrefix(line, "  - ") {
			// Line format: "  - Artist Name (N plays)"
			name := strings.TrimPrefix(line, "  - ")
			if idx := strings.LastIndex(name, " ("); idx > 0 {
				name = name[:idx]
			}
			return strings.TrimSpace(name)
		}
		if inTop && line != "" && !strings.HasPrefix(line, "  - ") {
			// Section ended
			break
		}
	}
	return ""
}

// hashProfile returns a 16-char hex prefix of SHA-256 of the profile text,
// used as a cache key for AI-generated playlists.
func hashProfile(profile string) string {
	h := sha256.Sum256([]byte(profile))
	return fmt.Sprintf("%x", h)[:16]
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[recommender] writeJSON encode: %v", err)
	}
}

// extractCountFromMessage parses track count from user messages like
// "30-track playlist", "50 songs", "20 tracks", "100 song playlist".
// Returns 0 if no count is found.
func extractCountFromMessage(msg string) int {
	re := regexp.MustCompile(`(\d+)\s*(?:[- ]?(?:track|song))`)
	matches := re.FindStringSubmatch(strings.ToLower(msg))
	if len(matches) >= 2 {
		n, _ := strconv.Atoi(matches[1])
		if n > 0 && n <= 100 {
			return n
		}
	}
	return 0
}

