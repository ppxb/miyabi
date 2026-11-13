package account

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

var ErrInvalidCredentials = domain.E(domain.KindUnauthorized, "用户名或密码错误", nil)

type credentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

// Store persists the single administrator account in the settings table.
type Store struct {
	db    *ent.Client
	mu    sync.RWMutex
	creds credentials
}

func New(ctx context.Context, db *ent.Client) (*Store, error) {
	store := &Store{db: db}
	saved, found, err := database.LoadSetting[credentials](ctx, db, domain.SettingAdminAccount)
	if err != nil {
		return nil, err
	}
	if found {
		store.creds = saved
	}
	return store, nil
}

func (s *Store) Configured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.creds.Username != "" && s.creds.PasswordHash != ""
}

func (s *Store) Username() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.creds.Username
}

// Verify checks the administrator username and password.
func (s *Store) Verify(username, password string) error {
	s.mu.RLock()
	current := s.creds
	s.mu.RUnlock()
	if current.Username == "" || current.PasswordHash == "" {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(current.Username), []byte(username)) != 1 {
		return ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(current.PasswordHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// SigningKey derives a stable JWT signing key from the stored password hash so
// sessions survive restarts and rotate only when the password changes.
func (s *Store) SigningKey() []byte {
	s.mu.RLock()
	hash := s.creds.PasswordHash
	s.mu.RUnlock()
	if hash == "" {
		return nil
	}
	sum := sha256.Sum256([]byte("miyabi-jwt:" + hash))
	return sum[:]
}

// Set validates and persists a new administrator credential.
func (s *Store) Set(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return domain.E(domain.KindInvalid, "用户名不能为空", nil)
	}
	if len(password) < 8 {
		return domain.E(domain.KindInvalid, "密码至少需要 8 位", nil)
	}
	return s.save(ctx, username, password)
}

func (s *Store) save(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return domain.E(domain.KindInvalid, "账号信息不完整", nil)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.E(domain.KindUnexpected, "无法保存管理员密码", err)
	}
	next := credentials{Username: username, PasswordHash: string(hash)}
	if err := database.SaveSetting(ctx, s.db, domain.SettingAdminAccount, next); err != nil {
		return err
	}
	s.mu.Lock()
	s.creds = next
	s.mu.Unlock()
	return nil
}
