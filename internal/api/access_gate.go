package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

var ErrAccessPassword = domain.E(domain.KindUnauthorized, "璁块棶瀵嗙爜閿欒", nil)

const defaultJWTSalt = "miyabi-jwt-signature-secret-v1"

// Account is the credential source behind the access gate. An empty username
// keeps the legacy password-only behavior.
type Account interface {
	Configured() bool
	Username() string
	Verify(username, password string) error
	SigningKey() []byte
}

// AccessGateService provides account verification and JWT token issuance/verification.
type AccessGateService struct {
	account   Account
	jwtSecret []byte
}

func NewAccessGateService(password, jwtSecret string) *AccessGateService {
	var secret []byte
	if jwtSecret != "" {
		secret = []byte(jwtSecret)
	} else if password != "" {
		mac := hmac.New(sha256.New, []byte(defaultJWTSalt))
		mac.Write([]byte(password))
		secret = mac.Sum(nil)
	}
	return &AccessGateService{
		account:   staticAccount{enabled: password != "", password: password},
		jwtSecret: secret,
	}
}

// NewAccessGateServiceWithAccount builds a gate backed by a persistent account.
func NewAccessGateServiceWithAccount(account Account, jwtSecret string) *AccessGateService {
	var secret []byte
	if jwtSecret != "" {
		secret = []byte(jwtSecret)
	}
	return &AccessGateService{account: account, jwtSecret: secret}
}

func (gate *AccessGateService) Enabled() bool {
	return gate.account.Configured()
}

// Verify keeps the legacy password-only contract used by older callers.
func (gate *AccessGateService) Verify(password string) error {
	return gate.account.Verify("", password)
}

// VerifyAccount verifies a username and password pair.
func (gate *AccessGateService) VerifyAccount(username, password string) error {
	return gate.account.Verify(username, password)
}

// AccountUsername returns the configured username, or an empty string in the
// password-only mode.
func (gate *AccessGateService) AccountUsername() string {
	return gate.account.Username()
}

func (gate *AccessGateService) GenerateToken() (string, int64, error) {
	if !gate.Enabled() {
		return "", 0, errors.New("access gate is disabled")
	}
	secret := gate.signingKey()
	if len(secret) == 0 {
		return "", 0, errors.New("access gate has no signing key")
	}
	now := time.Now().Unix()
	expiresAt := now + int64(defaultTokenTTL.Seconds())
	claims := jwtClaims{
		Subject:   "admin",
		Issuer:    "miyabi",
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}
	token, err := signToken(secret, claims)
	if err != nil {
		return "", 0, err
	}
	return token, expiresAt, nil
}

func (gate *AccessGateService) VerifyToken(token string) error {
	if !gate.Enabled() {
		return nil
	}
	_, err := verifyToken(gate.signingKey(), token)
	return err
}

func (gate *AccessGateService) signingKey() []byte {
	if len(gate.jwtSecret) > 0 {
		return gate.jwtSecret
	}
	return gate.account.SigningKey()
}

type staticAccount struct {
	enabled  bool
	password string
}

func (a staticAccount) Configured() bool { return a.enabled }

func (a staticAccount) Username() string { return "" }

func (a staticAccount) Verify(_ string, password string) error {
	if !a.enabled {
		return nil
	}
	actual := sha256.Sum256([]byte(password))
	expected := sha256.Sum256([]byte(a.password))
	if subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
		return ErrAccessPassword
	}
	return nil
}

func (a staticAccount) SigningKey() []byte {
	if !a.enabled {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(defaultJWTSalt))
	mac.Write([]byte(a.password))
	return mac.Sum(nil)
}
