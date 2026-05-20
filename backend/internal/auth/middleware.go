package auth

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func RequireAPIKey(next http.Handler) http.Handler {
	apiKey := os.Getenv("LEXICON_API_KEY")
	if apiKey == "" {
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
		if token != apiKey {
			log.Printf("[auth] invalid API key from %s", r.RemoteAddr)
			http.Error(w, `{"error":"invalid API key"}`, 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
