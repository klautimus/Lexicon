package auth

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
)

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
