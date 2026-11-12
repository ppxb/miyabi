package catalogue

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/javdb"
)

// JavDBConfig returns the optional account used for authenticated search.
func (service *Service) JavDBConfig(context.Context) (javdb.Config, error) {
	service.accountMu.RLock()
	defer service.accountMu.RUnlock()
	return service.javdbConfig, nil
}

// UpdateJavDBConfig persists the account and applies it to the live client.
func (service *Service) UpdateJavDBConfig(ctx context.Context, cfg javdb.Config) error {
	if err := cfg.Normalize(); err != nil {
		return err
	}
	service.accountMu.Lock()
	defer service.accountMu.Unlock()
	if err := database.SaveSetting(ctx, service.database, javdbAccountSetting, cfg); err != nil {
		return fmt.Errorf("save JavDB account: %w", err)
	}
	service.javdbConfig = cfg
	if service.javdbAccount != nil {
		service.javdbAccount.SetCredentials(cfg.Username, cfg.Password)
	}
	return nil
}

// TestJavDBLogin verifies the supplied account without persisting it.
func (service *Service) TestJavDBLogin(ctx context.Context, cfg javdb.Config) (javdb.LoginInfo, error) {
	if err := cfg.Normalize(); err != nil {
		return javdb.LoginInfo{}, err
	}
	if !cfg.Configured() {
		return javdb.LoginInfo{}, domain.E(domain.KindInvalid, "请填写 JavDB 用户名和密码", nil)
	}
	if service.javdbAccount == nil {
		return javdb.LoginInfo{}, domain.E(domain.KindInvalid, "JavDB 客户端不可用", nil)
	}
	return service.javdbAccount.Login(ctx, cfg.Username, cfg.Password)
}
