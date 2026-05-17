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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kevin/lexicon/internal/websearch"
)

type DeepSeekConfig struct {
	APIKey   string
	Model    string
	Thinking string
	BaseURL  string
}

type API struct {
	db  *sql.DB
	cfg DeepSeekConfig
	ws  *websearch.WebSearch
}

func New(db *sql.DB, cfg DeepSeekConfig, ws *websearch.WebSearch) *API {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	return &API{db: db, cfg: cfg, ws: ws}
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
	err := a.db.QueryRowContext(r.Context(),
		`SELECT payload, created_at FROM recommendations ORDER BY id DESC LIMIT 1`).Scan(&payload, &createdAt)
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

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	profile, err := a.buildProfile(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
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
				json.NewEncoder(w).Encode(out)
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

	prompt := `Given this user's listening profile, generate a cohesive playlist.
Return ONLY valid JSON with this exact shape:
{
  "name": "catchy playlist name",
  "description": "1-2 sentence vibe description",
  "tracks": [
    {"title":"...", "artist":"...", "reason":"brief why this fits"}
  ]
}

Rules:
- 8-12 tracks
- Name should be creative and thematic (not generic like "My Playlist")
- Mix of songs the user likely has and new discoveries
- Be specific and personal — reference patterns from the profile
- Output ONLY valid JSON, no prose, no code fences.
` + searchContext + `
PROFILE:
` + profile

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
	json.NewEncoder(w).Encode(out)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	profile, _ := a.buildProfile(ctx)

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

	system := `You are a music curator. ALWAYS respond with ONLY a single valid JSON object. No markdown, no prose outside the JSON, no code fences.

If the user asks for a playlist, use this shape:
{"message":"A short conversational reply","playlist":{"name":"Creative Playlist Name","description":"1-2 sentence vibe","tracks":[{"title":"...","artist":"...","reason":"..."}]}}

If the user asks a normal question (not about making a playlist), use this shape:
{"message":"Your concise answer here."}

Rules:
- For playlist requests: 8-12 tracks, creative thematic name, reference patterns from the profile.
- For normal questions: answer concisely in the "message" field.
- ALWAYS include the "message" field.
- NEVER output text outside the JSON object.
` + searchContext + `
---
USER LISTENING PROFILE:
` + profile + `
---
REMEMBER: Always return a JSON object with a "message" field.`
	reply, err := a.deepseekChatWithRetry(ctx, []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": req.Message},
	}, &dsRespFmt{Type: "json_object"}, func(raw string) error {
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
			rows.Scan(&a, &n)
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

func (a *API) callDeepSeek(ctx context.Context, profile string) (RecsPayload, error) {
	prompt := `You are a music & podcast recommendation engine. Based on the user's listening profile below, recommend 8 songs and 4 podcasts they might enjoy.

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
- Be specific — reference listening patterns from the profile
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
	json.Unmarshal([]byte(reply), &out)

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

type dsMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type dsRequest struct {
	Model           string      `json:"model"`
	Messages        []dsMessage `json:"messages"`
	Temperature     float64     `json:"temperature"`
	ThinkingEffort  string      `json:"thinking_effort,omitempty"`
	ResponseFormat  *dsRespFmt  `json:"response_format,omitempty"`
}
type dsRespFmt struct {
	Type string `json:"type"`
}

type dsResponse struct {
	Choices []struct {
		Message dsMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (a *API) deepseekChatWithRetry(
	ctx context.Context,
	msgs []map[string]string,
	respFmt *dsRespFmt,
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (a *API) deepseekChat(ctx context.Context, msgs []map[string]string, respFmt *dsRespFmt) (string, error) {
	dmsgs := make([]dsMessage, len(msgs))
	for i, m := range msgs {
		dmsgs[i] = dsMessage{Role: m["role"], Content: m["content"]}
	}
	reqBody := dsRequest{
		Model:          a.cfg.Model,
		Messages:       dmsgs,
		Temperature:    0.7,
		ResponseFormat: respFmt,
	}
	// thinking_effort is only valid for deepseek-reasoner models
	if strings.Contains(a.cfg.Model, "reasoner") && a.cfg.Thinking != "" {
		reqBody.ThinkingEffort = a.cfg.Thinking
	}
	buf, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("deepseek %d: %s", resp.StatusCode, string(body))
	}
	var dr dsResponse
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
	json.NewEncoder(w).Encode(v)
}
