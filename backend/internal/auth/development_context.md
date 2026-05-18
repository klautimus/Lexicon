# auth — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/auth/middleware.go` (27 LOC) — 🆕 NEW in v2
> **Last updated:** 2026-05-17

## Purpose

API key authentication middleware. Protects POST/PUT/DELETE endpoints with a Bearer token.

## Middleware

```go
func RequireAPIKey(next http.Handler) http.Handler
```

- Reads `LEXICON_API_KEY` env var
- If set, validates `Authorization: Bearer <token>` header
- If empty/unset, middleware is a no-op (no auth required)

## Usage

Mounted in `main.go` as chi middleware:
```go
r.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
            auth.RequireAPIKey(next).ServeHTTP(w, r)
            return
        }
        next.ServeHTTP(w, r)
    })
})
```

## Working Here

- Adding more auth methods: edit `middleware.go`
- Changing auth scope: edit the middleware mounting in `main.go`
