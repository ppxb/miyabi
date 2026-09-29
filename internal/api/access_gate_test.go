package api

import (
	"errors"
	"testing"
	"time"
)

func TestAccessGate(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		input    string
		valid    bool
	}{
		{name: "disabled", valid: true},
		{name: "disabled with input", input: "anything", valid: true},
		{name: "correct", password: "test-password", input: "test-password", valid: true},
		{name: "wrong", password: "test-password", input: "wrong"},
		{name: "empty", password: "test-password"},
		{name: "prefix", password: "test-password", input: "test"},
		{name: "case sensitive", password: "test-password", input: "TEST-PASSWORD"},
		{name: "unicode", password: "测试密码", input: "测试密码", valid: true},
		{name: "preserve whitespace", password: " test-password ", input: "test-password"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gate := NewAccessGateService(test.password, "")
			if gate.Enabled() != (test.password != "") {
				t.Fatal("unexpected gate state")
			}
			err := gate.Verify(test.input)
			if test.valid && err != nil {
				t.Fatalf("verify password: %v", err)
			}
			if !test.valid && !errors.Is(err, ErrAccessPassword) {
				t.Fatalf("expected password error, got %v", err)
			}
		})
	}
}

func TestAccessGateSessionPolicyAndStableKeys(t *testing.T) {
	for _, secret := range []string{"", "explicit-signing-key"} {
		t.Run(secret, func(t *testing.T) {
			gate := NewAccessGateService("password", secret)
			token, expiresAt, err := gate.GenerateToken()
			if err != nil {
				t.Fatal(err)
			}
			claims, err := verifyToken(gate.jwtSecret, token)
			if err != nil {
				t.Fatal(err)
			}
			if claims.Subject != "admin" || claims.Issuer != "miyabi" || claims.ExpiresAt != expiresAt ||
				claims.ExpiresAt-claims.IssuedAt != int64((30*24*time.Hour).Seconds()) {
				t.Fatalf("unexpected session claims: %+v", claims)
			}
			if err := NewAccessGateService("password", secret).VerifyToken(token); err != nil {
				t.Fatalf("recreated gate rejected existing session: %v", err)
			}
			if err := NewAccessGateService("password", "other-key").VerifyToken(token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("different key accepted session: %v", err)
			}
		})
	}
}

func TestDisabledAccessGateDoesNotIssueSessions(t *testing.T) {
	gate := NewAccessGateService("", "")
	if err := gate.VerifyToken("anything"); err != nil {
		t.Fatalf("disabled gate rejected request: %v", err)
	}
	if _, _, err := gate.GenerateToken(); err == nil {
		t.Fatal("disabled gate issued a session")
	}
}
