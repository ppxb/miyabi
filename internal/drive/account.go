package drive

import (
	"context"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	accountCacheTTL = 60 * time.Second
	// upstreamTimeout bounds shared upstream work that outlives an individual caller.
	upstreamTimeout = 45 * time.Second
)

// AccountStatus describes current 115 connection and mounted library directory.
type AccountStatus struct {
	Connected bool                     `json:"connected"`
	Account   *pan.Account             `json:"account,omitempty"`
	Directory *domain.LibraryDirectory `json:"directory,omitempty"`
}

func (d *Drive) verifyAccount(ctx context.Context, state snapshot) (pan.Account, error) {
	if err := ctx.Err(); err != nil {
		return pan.Account{}, err
	}
	if d.snapshot().closed {
		return pan.Account{}, context.Canceled
	}
	result := d.accountChecks.DoChan(fmt.Sprint(state.credentialVersion), func() (any, error) {
		done, ok := d.StartWork()
		if !ok {
			return pan.Account{}, context.Canceled
		}
		defer done()
		checkContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
		defer cancel()
		return d.loadAccount(checkContext, state)
	})
	select {
	case <-ctx.Done():
		return pan.Account{}, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return pan.Account{}, completed.Err
		}
		current, err := d.credentials(state)
		if err != nil {
			return pan.Account{}, err
		}
		if current.closed {
			return pan.Account{}, context.Canceled
		}
		return completed.Val.(pan.Account), nil
	}
}

// loadAccount checks the cache inside the shared lookup so a recently finished
// lookup cannot be followed by another fetch of the same credential version.
func (d *Drive) loadAccount(ctx context.Context, state snapshot) (pan.Account, error) {
	d.accountCacheMu.Lock()
	if _, err := d.credentials(state); err != nil {
		d.accountCacheMu.Unlock()
		return pan.Account{}, err
	}
	if d.cachedAccount.ID != "" && d.cachedCredentialVersion == state.credentialVersion && time.Since(d.cachedAccountTime) < accountCacheTTL {
		cached := d.cachedAccount
		d.accountCacheMu.Unlock()
		return cached, nil
	}
	d.accountCacheMu.Unlock()

	account, err := d.fetchAccount(ctx, state)
	if err != nil {
		return pan.Account{}, err
	}

	d.accountCacheMu.Lock()
	defer d.accountCacheMu.Unlock()
	if _, err := d.credentials(state); err != nil {
		return pan.Account{}, err
	}
	d.cachedAccount = account
	d.cachedAccountTime = time.Now()
	d.cachedCredentialVersion = state.credentialVersion
	return account, nil
}

func (d *Drive) invalidateAccountCache() {
	d.accountCacheMu.Lock()
	d.cachedAccount = pan.Account{}
	d.cachedAccountTime = time.Time{}
	d.cachedCredentialVersion = 0
	d.accountCacheMu.Unlock()
}

func (d *Drive) fetchAccount(ctx context.Context, state snapshot) (pan.Account, error) {
	account, err := withPanToken(ctx, d, state, func(current snapshot) (pan.Account, error) {
		return d.client.Account(ctx, current.tokens.AccessToken)
	})
	if err != nil {
		return pan.Account{}, fmt.Errorf("get 115 account: %w", err)
	}
	if _, err := d.credentials(state); err != nil {
		return pan.Account{}, err
	}
	return account, nil
}

func (d *Drive) Account(ctx context.Context) (AccountStatus, error) {
	state := d.snapshot()
	if state.tokens.AccessToken == "" {
		return AccountStatus{}, nil
	}
	account, err := d.verifyAccount(ctx, state)
	if err != nil {
		return AccountStatus{}, err
	}
	if err := d.commit.Lock(ctx); err != nil {
		return AccountStatus{}, err
	}
	defer d.commit.Unlock()
	if _, err := d.credentials(state); err != nil {
		return AccountStatus{}, err
	}
	if err := d.discardOtherAccountDirectory(ctx, account.ID); err != nil {
		return AccountStatus{}, err
	}
	status := AccountStatus{Connected: true, Account: &account}
	directory := d.snapshot().directory
	if directory.AccountID == account.ID {
		value := directory.LibraryDirectory
		status.Directory = &value
	}
	return status, nil
}

func (d *Drive) acceptLogin(ctx context.Context, session *loginSession, tokens pan.Tokens) error {
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	d.mu.Lock()
	current := d.login.session == session
	d.mu.Unlock()
	if !current {
		d.commit.Unlock()
		return nil
	}
	if err := database.SaveSetting(ctx, d.database, credentialsSetting, tokens); err != nil {
		d.commit.Unlock()
		return err
	}
	d.mu.Lock()
	d.tokens = tokens
	d.tokenVersion++
	d.credentialVersion++
	d.authorizationVersion++
	d.mu.Unlock()
	d.invalidateAccountCache()
	state := d.snapshot()
	d.commit.Unlock()

	account, err := d.fetchAccount(ctx, state)
	if err != nil {
		return fmt.Errorf("verify 115 login account: %w", err)
	}
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	if _, err := d.credentials(state); err != nil {
		return err
	}
	return d.discardOtherAccountDirectory(ctx, account.ID)
}

func (d *Drive) Disconnect(ctx context.Context) (AccountStatus, error) {
	if err := d.commit.Lock(ctx); err != nil {
		return AccountStatus{}, err
	}
	defer d.commit.Unlock()
	if err := d.clearCredentials(ctx); err != nil {
		return AccountStatus{}, err
	}
	d.mu.Lock()
	d.login.session = nil
	d.mu.Unlock()
	return AccountStatus{}, nil
}

// clearCredentials requires the commit lock. Keep the mount and any pending QR
// login so rejected credentials can be replaced without resetting the library.
func (d *Drive) clearCredentials(ctx context.Context) error {
	if _, err := d.database.Setting.Delete().Where(setting.Key(credentialsSetting)).Exec(ctx); err != nil {
		return fmt.Errorf("remove 115 credentials: %w", err)
	}
	d.mu.Lock()
	d.tokens = pan.Tokens{}
	d.credentialVersion++
	d.authorizationVersion++
	d.tokenVersion++
	d.mu.Unlock()
	d.invalidateAccountCache()
	return nil
}
