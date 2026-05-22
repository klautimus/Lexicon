package auth

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
)

// ─── API key auth (kept for backward compatibility) ──────────────────────────

// apiKey is read once at startup from the LEXICON_API_KEY environment variable.
// An empty value means auth is disabled (all requests pass through unauthenticated).
var apiKey string

// SetAPIKey sets the API key from the environment. Must be called after
// godotenv.Load() so that .env values are available.
func SetAPIKey() {
	apiKey = os.Getenv("LEXICON_API_KEY")
}

// KeyIsSet returns true when an API key has been configured.
func KeyIsSet() bool {
	return apiKey != ""
}

// KeyLen returns the configured key length (0 if unset).
func KeyLen() int {
	return len(apiKey)
}

func RequireAPIKey(next http.Handler) http.Handler {
	if apiKey == "" {
		log.Printf("[auth] WARNING: LEXICON_API_KEY is not set — all endpoints are unauthenticated")
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			log.Printf("[auth] missing Authorization header from %s", r.RemoteAddr)
			http.Error(w, `{"error":"missing Authorization header"}`, 401)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
			log.Printf("[auth] invalid API key from %s", r.RemoteAddr)
			http.Error(w, `{"error":"invalid API key"}`, 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Context helpers ─────────────────────────────────────────────────────────

type contextKey string

const userContextKey contextKey = "user"

// UserFromContext extracts the authenticated user from the request context.
// Returns nil if no user is present.
func UserFromContext(ctx context.Context) (*UserInfo, bool) {
	u, ok := ctx.Value(userContextKey).(*UserInfo)
	return u, ok
}

// ContextWithUser returns a new context with the given user identity embedded.
// Use this when injecting an authenticated user into a background context
// (e.g. syncer workers that iterate over multiple users).
func ContextWithUser(ctx context.Context, u *UserInfo) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// ─── Session middleware ──────────────────────────────────────────────────────

// extractToken pulls a session token from the Authorization header or the
// "lexicon_session" cookie.
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if c, err := r.Cookie("lexicon_session"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// RequireAuth is chi middleware that enforces authentication. It first tries
// a session token (Bearer header or cookie), then falls through to the
// configured API key. If neither succeeds the request is rejected with 401.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)

		// 1. Try session token.
		if token != "" {
			if u := GetSession(token); u != nil {
				ctx := ContextWithUser(r.Context(), u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// 2. Fall through to API key if configured.
		if apiKey != "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key := strings.TrimPrefix(auth, "Bearer ")
				if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
					// API key auth — no specific user identity; handlers
					// will treat this as unfiltered (backward compat).
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// 3. If API key is NOT configured, pass through unauthenticated
		// (desktop app backward compatibility).
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, `{"error":"unauthorized"}`, 401)
	})
}

// RequireAdmin is additional middleware that rejects non-admin users.
// Must be used after RequireAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok || u.Role != "admin" {
			http.Error(w, `{"error":"admin required"}`, 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}
