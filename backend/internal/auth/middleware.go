package auth

import (
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
			http.Error(w, `{"error":"missing Authorization header"}`, 401)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != apiKey {
			http.Error(w, `{"error":"invalid API key"}`, 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
