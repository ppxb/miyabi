package drive

import (
	"context"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	// upstreamTimeout bounds shared upstream work that outlives an individual caller.
	upstreamTimeout = 45 * time.Second
)

// AccountStatus describes one 115 account and its mounted library directory.
type AccountStatus struct {
	AccountID string                   `json:"account_id,omitempty"`
	Connected bool                     `json:"connected"`
	Account   *pan.Account             `json:"account,omitempty"`
	Directory *domain.LibraryDirectory `json:"directory,omitempty"`
}

// acceptLogin stores a freshly authorized account into the account registry. A
// later directory selection is a separate, per-account step.
func (d *Drive) acceptLogin(ctx context.Context, session *loginSession, tokens pan.Tokens) error {
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	d.mu.Lock()
	current := d.login.session == session
	d.mu.Unlock()
	if !current {
		return nil
	}
	account, err := d.client.Account(ctx, tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("verify 115 login account: %w", err)
	}
	if err := d.saveAccountTokens(ctx, account.ID, tokens); err != nil {
		return err
	}
	d.bumpAccountCredential(account.ID)
	d.bumpAccountAuthorization(account.ID)
	d.mu.Lock()
	session.accountID = account.ID
	d.mu.Unlock()
	return nil
}
