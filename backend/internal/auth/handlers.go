package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handler provides HTTP endpoints for authentication and user management.
type Handler struct {
	db *sql.DB
}

// NewHandler creates a new auth handler backed by the given database.
func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// Mount registers auth routes on the chi router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/api/auth", func(r chi.Router) {
		// Public endpoints (no auth required)
		r.Post("/login", h.login)

		// Protected endpoints (require valid session)
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth)
			r.Post("/logout", h.logout)
			r.Get("/me", h.me)
		})

		// Admin-only endpoints
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth)
			r.Use(RequireAdmin)
			r.Post("/users", h.createUser)
			r.Get("/users", h.listUsers)
			r.Delete("/users/{id}", h.deleteUser)
		})
	})
}

// ─── Request / response types ───────────────────────────────────────────────

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResp struct {
	Token string   `json:"token"`
	User  UserInfo `json:"user"`
}

type createUserReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type userRow struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	CreatedAt   int64  `json:"created_at"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	// First-run detection: if the users table is empty, create an admin
	// account with the provided credentials.
	var userCount int
	if err := h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		log.Printf("[auth] login count users: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if userCount == 0 {
		// First run — create admin user
		hash, err := HashPassword(req.Password)
		if err != nil {
			log.Printf("[auth] first-run hash: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		res, err := h.db.ExecContext(r.Context(),
			`INSERT INTO users (username, password_hash, display_name, role) VALUES (?, ?, '', 'admin')`,
			req.Username, hash)
		if err != nil {
			log.Printf("[auth] first-run insert: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		userID, _ := res.LastInsertId()

		u := UserInfo{UserID: userID, Username: req.Username, DisplayName: "", Role: "admin", IsAdmin: true}
		token, err := GenerateSessionToken()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		SetSession(token, u)
		writeJSON(w, http.StatusOK, loginResp{Token: token, User: u})
		return
	}

	// Normal authentication: look up user by username, compare password hash.
	var userID int64
	var role, passwordHash, displayName string
	err := h.db.QueryRowContext(r.Context(),
		`SELECT id, password_hash, display_name, role FROM users WHERE username = ?`,
		req.Username).Scan(&userID, &passwordHash, &displayName, &role)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if err != nil {
		log.Printf("[auth] login query: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if !CheckPassword(passwordHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	u := UserInfo{UserID: userID, Username: req.Username, DisplayName: displayName, Role: role, IsAdmin: role == "admin"}
	token, err := GenerateSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	SetSession(token, u)
	writeJSON(w, http.StatusOK, loginResp{Token: token, User: u})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token != "" {
		DeleteSession(token)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be 'admin' or 'user'"})
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		log.Printf("[auth] createUser hash: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	res, err := h.db.ExecContext(r.Context(),
		`INSERT INTO users (username, password_hash, display_name, role) VALUES (?, ?, ?, ?)`,
		req.Username, hash, req.DisplayName, req.Role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
			return
		}
		log.Printf("[auth] createUser insert: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"user": userRow{
		ID:          id,
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Role:        req.Role,
	}})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, username, display_name, role, created_at FROM users ORDER BY id`)
	if err != nil {
		log.Printf("[auth] listUsers query: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer rows.Close()

	var out []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt); err != nil {
			log.Printf("[auth] listUsers scan: %v", err)
			continue
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[auth] listUsers rows: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if out == nil {
		out = []userRow{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	// Parse the target user ID from the URL.
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing user id"})
		return
	}
	var targetID int64
	if _, err := fmt.Sscanf(idStr, "%d", &targetID); err != nil || targetID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	// Prevent self-deletion.
	if caller, ok := UserFromContext(r.Context()); ok && caller.UserID == targetID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cannot delete yourself"})
		return
	}

	res, err := h.db.ExecContext(r.Context(), `DELETE FROM users WHERE id = ?`, targetID)
	if err != nil {
		log.Printf("[auth] deleteUser delete: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[auth] writeJSON encode: %v", err)
	}
}
