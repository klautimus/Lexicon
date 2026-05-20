package apple

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
)

// TestSignDeveloperToken_RoundTrip generates an ephemeral P-256 key, signs a
// fake developer token, then verifies the resulting JWT against the same key.
// This exercises the ASN.1 -> raw R||S conversion that Apple's ES256 needs.
func TestSignDeveloperToken_RoundTrip(t *testing.T) {
	// Generate an ephemeral P-256 key.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	teamID := "ABCDEFGHIJ"
	keyID := "0123456789"
	tok, exp, err := signDeveloperToken(teamID, keyID, string(pemBytes))
	if err != nil {
		t.Fatalf("signDeveloperToken: %v", err)
	}
	if exp <= 0 {
		t.Fatalf("expected positive exp, got %d", exp)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token should have 3 parts, got %d (%q)", len(parts), tok)
	}
	signingInput := parts[0] + "." + parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sig) != 64 {
		t.Fatalf("P-256 raw signature should be 64 bytes, got %d", len(sig))
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	digest := sha256.Sum256([]byte(signingInput))
	if !ecdsa.Verify(&priv.PublicKey, digest[:], r, s) {
		t.Fatalf("signature did not verify against the same key")
	}

	// Verify header claims look right.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if !strings.Contains(string(headerJSON), `"alg":"ES256"`) {
		t.Fatalf("header missing ES256 alg: %s", headerJSON)
	}
	if !strings.Contains(string(headerJSON), `"kid":"0123456789"`) {
		t.Fatalf("header missing kid: %s", headerJSON)
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if !strings.Contains(string(claimsJSON), `"iss":"ABCDEFGHIJ"`) {
		t.Fatalf("claims missing iss: %s", claimsJSON)
	}
}

func TestSignDeveloperToken_BadInputs(t *testing.T) {
	cases := []struct {
		name, team, key, pem, want string
	}{
		{"short team", "ABC", "0123456789", "x", "team_id must be 10"},
		{"short key", "ABCDEFGHIJ", "012", "x", "key_id must be 10"},
		{"empty pem", "ABCDEFGHIJ", "0123456789", "", "empty private key"},
		{"garbage pem", "ABCDEFGHIJ", "0123456789", "not a pem", "not PEM-encoded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := signDeveloperToken(c.team, c.key, c.pem)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q did not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestParseP8_RSARejected(t *testing.T) {
	// Build an RSA-shaped PEM and confirm we reject it.
	rsaPEM := `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQCxsXSL5T1Q3CWY
-----END PRIVATE KEY-----`
	_, err := parseP8PrivateKey(rsaPEM)
	if err == nil {
		t.Fatalf("expected RSA key to be rejected")
	}
}
