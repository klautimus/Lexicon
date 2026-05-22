package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// UserInfo holds the authenticated user's identity, injected into the
// request context by the session middleware.
type UserInfo struct {
	UserID      int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	IsAdmin     bool   `json:"is_admin"`
}

type session struct {
	User      UserInfo
	ExpiresAt time.Time
}

// Sessions are stored in-memory (sync.Map). This is an intentional design choice
// for a desktop app: server restarts are infrequent, and in-memory lookups are
// faster than SQLite-backed sessions. If Lexicon ever becomes a multi-instance
// server app, sessions should be moved to the database for persistence across
// restarts and horizontal scaling.
var (
	sessionStore    sync.Map // token string -> *session
	sessionDuration = 24 * time.Hour
)

// GenerateSessionToken returns a random 32-byte hex string suitable for use
// as a session token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetSession stores a user session under the given token with a 24-hour
// expiry.
func SetSession(token string, u UserInfo) {
	sessionStore.Store(token, &session{
		User:      u,
		ExpiresAt: time.Now().Add(sessionDuration),
	})
}

// GetSession looks up a session by token. Returns nil if the token is
// unknown or the session has expired (expired sessions are cleaned up on
// access).
func GetSession(token string) *UserInfo {
	v, ok := sessionStore.Load(token)
	if !ok {
		return nil
	}
	s := v.(*session)
	if time.Now().After(s.ExpiresAt) {
		sessionStore.Delete(token)
		return nil
	}
	return &s.User
}

// DeleteSession removes a session from the store (logout).
func DeleteSession(token string) {
	sessionStore.Delete(token)
}

// CleanupSessions removes all expired sessions. Call periodically from a
// background goroutine.
func CleanupSessions() {
	sessionStore.Range(func(key, value interface{}) bool {
		s := value.(*session)
		if time.Now().After(s.ExpiresAt) {
			sessionStore.Delete(key)
		}
		return true
	})
}

// StartSessionCleanup launches a goroutine that prunes expired sessions
// every interval and runs until ctx is cancelled.
func StartSessionCleanup(interval time.Duration) (stop func()) {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				CleanupSessions()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	return func() { close(done) }
}
