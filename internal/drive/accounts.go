package drive

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	accountsSetting      = "pan.accounts"
	accountRefreshTick   = 10 * time.Minute
	accountRefreshWindow = 30 * time.Minute
)

// accountRecord keeps one 115 account's tokens and mounted directory so several
// accounts can be listed and, in later steps, scanned independently.
type accountRecord struct {
	Tokens    pan.Tokens  `json:"tokens"`
	Directory mountRecord `json:"directory"`
}

type accountStore map[string]accountRecord

func (d *Drive) loadAccounts(ctx context.Context) (accountStore, error) {
	store, _, err := database.LoadSetting[accountStore](ctx, d.database, accountsSetting)
	if err != nil {
		return nil, err
	}
	if store == nil {
		store = accountStore{}
	}
	return store, nil
}

func (d *Drive) saveAccountTokens(ctx context.Context, accountID string, tokens pan.Tokens) error {
	if accountID == "" {
		return nil
	}
	d.accountsMu.Lock()
	defer d.accountsMu.Unlock()
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return err
	}
	record := store[accountID]
	record.Tokens = tokens
	store[accountID] = record
	d.setAccountToken(accountID, tokens)
	return database.SaveSetting(ctx, d.database, accountsSetting, store)
}

func (d *Drive) accountToken(accountID string) pan.Tokens {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.accountTokens[accountID]
}

func (d *Drive) setAccountToken(accountID string, tokens pan.Tokens) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.accountTokens == nil {
		d.accountTokens = make(map[string]pan.Tokens)
	}
	d.accountTokens[accountID] = tokens
}

func (d *Drive) accountCredVersion(accountID string) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.accountCredVersions[accountID]
}

func (d *Drive) accountAuthVersion(accountID string) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.accountAuthVersions[accountID]
}

// bumpAccountCredential invalidates commits that were authorized by older credentials.
func (d *Drive) bumpAccountCredential(accountID string) {
	if accountID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.accountCredVersions == nil {
		d.accountCredVersions = make(map[string]uint64)
	}
	d.accountCredVersions[accountID]++
}

// bumpAccountAuthorization invalidates reads tied to an older mount or login.
func (d *Drive) bumpAccountAuthorization(accountID string) {
	if accountID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.accountAuthVersions == nil {
		d.accountAuthVersions = make(map[string]uint64)
	}
	d.accountAuthVersions[accountID]++
}

func (d *Drive) saveAccountMount(ctx context.Context, accountID string, mount mountRecord) error {
	if accountID == "" {
		return nil
	}
	d.accountsMu.Lock()
	defer d.accountsMu.Unlock()
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return err
	}
	record := store[accountID]
	record.Directory = mount
	store[accountID] = record
	d.bumpAccountAuthorization(accountID)
	return database.SaveSetting(ctx, d.database, accountsSetting, store)
}

func (d *Drive) removeAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	d.accountsMu.Lock()
	defer d.accountsMu.Unlock()
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return err
	}
	if _, ok := store[accountID]; !ok {
		return nil
	}
	delete(store, accountID)
	d.setAccountToken(accountID, pan.Tokens{})
	d.bumpAccountCredential(accountID)
	d.bumpAccountAuthorization(accountID)
	return database.SaveSetting(ctx, d.database, accountsSetting, store)
}

func (d *Drive) accountRecord(ctx context.Context, accountID string) (accountRecord, error) {
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return accountRecord{}, err
	}
	return store[accountID], nil
}

// Mount returns the stored library source of an account when it has one. A
// missing mount reports ok=false with a nil error; a read failure surfaces err.
func (d *Drive) Mount(ctx context.Context, accountID string) (domain.LibrarySource, bool, error) {
	record, err := d.accountRecord(ctx, accountID)
	if err != nil {
		return domain.LibrarySource{}, false, err
	}
	if record.Directory.ID == "" || record.Directory.AccountID == "" {
		return domain.LibrarySource{}, false, nil
	}
	return record.Directory.source(), true, nil
}

// Mounts returns every stored account's mounted library source, without
// verifying credentials, so callers can operate each account independently.
func (d *Drive) Mounts(ctx context.Context) ([]domain.LibrarySource, error) {
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.LibrarySource, 0, len(store))
	for _, record := range store {
		if record.Directory.ID == "" || record.Directory.AccountID == "" {
			continue
		}
		result = append(result, record.Directory.source())
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AccountID != result[j].AccountID {
			return result[i].AccountID < result[j].AccountID
		}
		return result[i].Directory.ID < result[j].Directory.ID
	})
	return result, nil
}

// Accounts lists every stored 115 account and its mounted directory.
func (d *Drive) Accounts(ctx context.Context) ([]AccountStatus, error) {
	store, err := d.loadAccounts(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(store))
	for id := range store {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]AccountStatus, 0, len(ids))
	for _, id := range ids {
		record := store[id]
		if tokensNeedRefresh(record.Tokens) {
			if err := d.refreshAccountTokens(ctx, id); err != nil {
				slog.Warn("refresh 115 account tokens", "account", id, "error", err)
				result = append(result, AccountStatus{AccountID: id, Connected: false})
				continue
			}
			record.Tokens = d.accountToken(id)
		}
		account, err := d.client.Account(ctx, record.Tokens.AccessToken)
		if err != nil {
			result = append(result, AccountStatus{AccountID: id, Connected: false})
			continue
		}
		status := AccountStatus{AccountID: id, Connected: true, Account: &account}
		if record.Directory.ID != "" && record.Directory.AccountID == account.ID {
			directory := record.Directory.LibraryDirectory
			status.Directory = &directory
		}
		result = append(result, status)
	}
	return result, nil
}

func tokensNeedRefresh(tokens pan.Tokens) bool {
	if tokens.RefreshToken == "" || tokens.ExpiresAt.IsZero() {
		return false
	}
	return time.Until(tokens.ExpiresAt) <= accountRefreshWindow
}

// StartAccountRefresh periodically refreshes expiring tokens of stored accounts.
func (d *Drive) StartAccountRefresh(ctx context.Context) {
	d.mu.Lock()
	stop := d.refreshStop
	d.mu.Unlock()
	if stop == nil {
		return
	}
	done, ok := d.StartWork()
	if !ok {
		return
	}
	go func() {
		defer done()
		ticker := time.NewTicker(accountRefreshTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				d.refreshExpiringAccounts(ctx)
			}
		}
	}()
}

func (d *Drive) refreshExpiringAccounts(ctx context.Context) {
	store, err := d.loadAccounts(ctx)
	if err != nil {
		slog.Warn("load 115 accounts for refresh", "error", err)
		return
	}
	for id, record := range store {
		if tokensNeedRefresh(record.Tokens) {
			if err := d.refreshAccountTokens(ctx, id); err != nil {
				slog.Warn("refresh 115 account tokens", "account", id, "error", err)
			}
		}
	}
}
