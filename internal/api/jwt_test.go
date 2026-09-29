package api

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJWT_SignAndVerify(t *testing.T) {
	secret := []byte("super-secret-key-32-bytes-long!")
	now := time.Now().Unix()

	claims := jwtClaims{
		Subject:   "admin",
		Issuer:    "miyabi",
		IssuedAt:  now,
		ExpiresAt: now + 3600,
	}

	token, err := signToken(secret, claims)
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	if token == "" {
		t.Fatal("generated token is empty")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}

	verified, err := verifyToken(secret, token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if verified.Subject != "admin" {
		t.Errorf("expected subject 'admin', got %s", verified.Subject)
	}
	if verified.Issuer != "miyabi" {
		t.Errorf("expected issuer 'miyabi', got %s", verified.Issuer)
	}
	if verified.ExpiresAt != claims.ExpiresAt {
		t.Errorf("expected expires_at %d, got %d", claims.ExpiresAt, verified.ExpiresAt)
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	secret := []byte("secret")
	now := time.Now().Unix()

	claims := jwtClaims{
		Subject:   "admin",
		Issuer:    "miyabi",
		IssuedAt:  now - 7200,
		ExpiresAt: now - 3600, // Expired 1 hour ago
	}

	token, err := signToken(secret, claims)
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	_, err = verifyToken(secret, token)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestJWT_TamperedSignature(t *testing.T) {
	secret := []byte("secret")
	claims := jwtClaims{
		Subject:   "admin",
		ExpiresAt: time.Now().Unix() + 3600,
	}

	token, err := signToken(secret, claims)
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	tampered := token + "tampered"
	_, err = verifyToken(secret, tampered)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWT_TamperedPayload(t *testing.T) {
	secret := []byte("secret")
	claims := jwtClaims{
		Subject:   "admin",
		ExpiresAt: time.Now().Unix() + 3600,
	}

	token, err := signToken(secret, claims)
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	parts := strings.Split(token, ".")
	tampered := parts[0] + ".eyJzdWIiOiJoYWNrZXIifQ." + parts[2]
	_, err = verifyToken(secret, tampered)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWT_WrongSecret(t *testing.T) {
	secret1 := []byte("secret1")
	secret2 := []byte("secret2")

	claims := jwtClaims{
		Subject:   "admin",
		ExpiresAt: time.Now().Unix() + 3600,
	}

	token, err := signToken(secret1, claims)
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}

	_, err = verifyToken(secret2, token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWT_MalformedToken(t *testing.T) {
	secret := []byte("secret")

	for _, malformed := range []string{
		"",
		"part1.part2",
		"part1.part2.part3.part4",
		"invalid-base64.invalid-base64.invalid-base64",
	} {
		_, err := verifyToken(secret, malformed)
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("for input %q: expected ErrInvalidToken, got %v", malformed, err)
		}
	}
}
