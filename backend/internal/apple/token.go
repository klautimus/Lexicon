// Package apple implements Apple Music API integration for Lexicon.
//
// The auth model is dual-token:
//
//  1. Developer Token — server-side JWT signed with ES256 using the .p8
//     private key from the Apple Developer portal. Lifetime up to 6 months.
//     We mint and cache it; refresh ~24h before expiry.
//
//  2. Music User Token (MUT) — browser-issued via MusicKit JS in the
//     Lexicon Settings page, then POSTed to the backend and stored in
//     apple_music_user.music_user_token. No server-side refresh exists; if
//     Apple invalidates the MUT, the user re-authorizes in the Settings page.
//
// Credentials are entered through the Settings GUI and stored in
// apple_music_config. There is no .env requirement.
package apple

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// Token lifetime: 90 days. Apple allows up to 15777000s (~6 months). We use
// a shorter window so a long-running daemon refreshes more often and a
// compromised token has a smaller blast radius.
const (
	devTokenLifetime = 90 * 24 * time.Hour
	// Refresh once within 24h of expiry.
	devTokenRefreshWindow = 24 * time.Hour
)

// tokenMintMu prevents two concurrent mint+cache cycles racing on the DB.
var tokenMintMu sync.Mutex

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtClaims struct {
	Iss string `json:"iss"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

// MintDeveloperToken returns a valid Apple Music developer token. It uses
// the cached token in apple_music_config if it has more than
// devTokenRefreshWindow remaining; otherwise it mints, caches, and returns
// a fresh token. Returns ErrNotConfigured if credentials are not yet saved.
func MintDeveloperToken(ctx context.Context, db *sql.DB) (string, error) {
	tokenMintMu.Lock()
	defer tokenMintMu.Unlock()

	var (
		teamID, keyID, privateKey, cached string
		expiresAt                         int64
	)
	row := db.QueryRowContext(ctx, `
		SELECT team_id, key_id, private_key,
		       IFNULL(cached_dev_token,''), IFNULL(cached_dev_token_expires_at, 0)
		FROM apple_music_config WHERE id=1`)
	if err := row.Scan(&teamID, &keyID, &privateKey, &cached, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotConfigured
		}
		return "", fmt.Errorf("read apple_music_config: %w", err)
	}

	now := time.Now().Unix()
	if cached != "" && expiresAt-now > int64(devTokenRefreshWindow.Seconds()) {
		return cached, nil
	}

	tok, exp, err := signDeveloperToken(teamID, keyID, privateKey)
	if err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE apple_music_config SET cached_dev_token=?, cached_dev_token_expires_at=?, updated_at=? WHERE id=1`,
		tok, exp, time.Now().Unix()); err != nil {
		return "", fmt.Errorf("cache dev token: %w", err)
	}
	return tok, nil
}

// ErrNotConfigured is returned when the Apple Music credentials have not been
// saved to apple_music_config yet (user hasn't completed Settings → Apple Music
// → Save).
var ErrNotConfigured = errors.New("apple music not configured")

// signDeveloperToken builds and signs a fresh JWT. Returns the token and its
// absolute expiry (unix seconds).
func signDeveloperToken(teamID, keyID, pemKey string) (string, int64, error) {
	if len(teamID) != 10 {
		return "", 0, fmt.Errorf("team_id must be 10 characters, got %d", len(teamID))
	}
	if len(keyID) != 10 {
		return "", 0, fmt.Errorf("key_id must be 10 characters, got %d", len(keyID))
	}
	priv, err := parseP8PrivateKey(pemKey)
	if err != nil {
		return "", 0, fmt.Errorf("parse private key: %w", err)
	}

	now := time.Now()
	exp := now.Add(devTokenLifetime)
	headerJSON, err := json.Marshal(jwtHeader{Alg: "ES256", Kid: keyID})
	if err != nil {
		return "", 0, err
	}
	claimsJSON, err := json.Marshal(jwtClaims{Iss: teamID, Iat: now.Unix(), Exp: exp.Unix()})
	if err != nil {
		return "", 0, err
	}

	signingInput := base64URLEncode(headerJSON) + "." + base64URLEncode(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))

	// ecdsa.Sign returns r and s as *big.Int. Apple's JWS ES256 expects raw
	// fixed-width R||S concatenation (32+32 bytes for P-256), NOT ASN.1 DER.
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return "", 0, fmt.Errorf("ecdsa sign: %w", err)
	}
	curveByteSize := (priv.Curve.Params().BitSize + 7) / 8 // 32 for P-256
	sig := make([]byte, 2*curveByteSize)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[curveByteSize-len(rBytes):curveByteSize], rBytes)
	copy(sig[2*curveByteSize-len(sBytes):], sBytes)

	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
	return token, exp.Unix(), nil
}

// parseP8PrivateKey parses a PEM-encoded PKCS#8 ECDSA private key. The .p8
// file Apple provides is exactly this format; we accept the full PEM contents
// (including header/footer lines) pasted into the Settings textarea.
func parseP8PrivateKey(pemKey string) (*ecdsa.PrivateKey, error) {
	pemKey = strings.TrimSpace(pemKey)
	if pemKey == "" {
		return nil, errors.New("empty private key")
	}
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("not PEM-encoded — expected -----BEGIN PRIVATE KEY----- header")
	}
	// Apple .p8 files are PKCS#8. Try PKCS#8 first, then EC fall-back.
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key is %T, expected *ecdsa.PrivateKey", k)
		}
		if ec.Curve.Params().BitSize != 256 {
			return nil, fmt.Errorf("curve is %s, expected P-256", ec.Curve.Params().Name)
		}
		return ec, nil
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		if k.Curve.Params().BitSize != 256 {
			return nil, fmt.Errorf("curve is %s, expected P-256", k.Curve.Params().Name)
		}
		return k, nil
	}
	return nil, errors.New("could not parse as PKCS#8 or EC private key")
}

// ValidateConfig parses + signs against the given credentials to verify they
// produce a valid token. Used by POST /api/apple/config before persisting.
func ValidateConfig(teamID, keyID, pemKey string) error {
	_, _, err := signDeveloperToken(teamID, keyID, pemKey)
	return err
}

// fastValidateBigInt is currently unused but kept to assert big.Int import
// is meaningful if signDeveloperToken's signature math ever changes.
var _ = (*big.Int)(nil)

// base64URLEncode is base64.RawURLEncoding-encoded JSON byte slice as string.
func base64URLEncode(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
