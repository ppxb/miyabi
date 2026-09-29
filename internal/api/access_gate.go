package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

var ErrAccessPassword = domain.E(domain.KindUnauthorized, "访问密码错误", nil)

const defaultJWTSalt = "miyabi-jwt-signature-secret-v1"

// AccessGateService provides password verification and JWT token issuance/verification.
type AccessGateService struct {
	enabled      bool
	passwordHash [sha256.Size]byte
	jwtSecret    []byte
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
		enabled:      password != "",
		passwordHash: sha256.Sum256([]byte(password)),
		jwtSecret:    secret,
	}
}

func (gate *AccessGateService) Enabled() bool {
	return gate.enabled
}

func (gate *AccessGateService) Verify(password string) error {
	if !gate.enabled {
		return nil
	}
	actual := sha256.Sum256([]byte(password))
	if subtle.ConstantTimeCompare(gate.passwordHash[:], actual[:]) != 1 {
		return ErrAccessPassword
	}
	return nil
}

func (gate *AccessGateService) GenerateToken() (string, int64, error) {
	if !gate.enabled {
		return "", 0, errors.New("access gate is disabled")
	}
	now := time.Now().Unix()
	expiresAt := now + int64(defaultTokenTTL.Seconds())
	claims := jwtClaims{
		Subject:   "admin",
		Issuer:    "miyabi",
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}
	token, err := signToken(gate.jwtSecret, claims)
	if err != nil {
		return "", 0, err
	}
	return token, expiresAt, nil
}

func (gate *AccessGateService) VerifyToken(token string) error {
	if !gate.enabled {
		return nil
	}
	_, err := verifyToken(gate.jwtSecret, token)
	return err
}
