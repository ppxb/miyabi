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
	d.accountTokens[accountID] = tokens
}

func (d *Drive) accountMount(accountID string) (mountRecord, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	record, ok := d.accountMounts[accountID]
	return record, ok
}

func (d *Drive) setAccountMount(accountID string, mount mountRecord) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.accountMounts[accountID] = mount
}

func (d *Drive) clearAccountMount(accountID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.accountMounts, accountID)
}

// syncAccountMounts replaces the in-memory mount registry with the given stored
// accounts, so periodic refreshes pick up any external/manual DB edit.
func (d *Drive) syncAccountMounts(store accountStore) {
	d.mu.Lock()
	defer d.mu.Unlock()
	next := make(map[string]mountRecord, len(store))
	for id, record := range store {
		next[id] = record.Directory
	}
	d.accountMounts = next
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
	if err := database.SaveSetting(ctx, d.database, accountsSetting, store); err != nil {
		return err
	}
	d.setAccountMount(accountID, mount)
	return nil
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
	if err := database.SaveSetting(ctx, d.database, accountsSetting, store); err != nil {
		return err
	}
	d.clearAccountMount(accountID)
	return nil
}

// accountRecord builds one account record from the in-memory registry. Tokens
// and mounts are loaded at construction and kept in sync by the write paths, so
// callers such as OpenSource/accountSessionFor no longer reload the whole store.
func (d *Drive) accountRecord(_ context.Context, accountID string) (accountRecord, error) {
	mount, _ := d.accountMount(accountID)
	return accountRecord{Tokens: d.accountToken(accountID), Directory: mount}, nil
}

// Mount returns the stored library source of an account when it has one. A
// missing mount reports ok=false with a nil error. The mount is served from the
// in-memory registry (kept in sync by saveAccountMount/removeAccount) so remote
// operations do not reload and deserialize the whole account store.
func (d *Drive) Mount(_ context.Context, accountID string) (domain.LibrarySource, bool, error) {
	record, ok := d.accountMount(accountID)
	if !ok || record.ID == "" || record.AccountID == "" {
		return domain.LibrarySource{}, false, nil
	}
	return record.source(), true, nil
}

// Mounts returns every stored account's mounted library source, without
// verifying credentials, so callers can operate each account independently.
func (d *Drive) Mounts(_ context.Context) ([]domain.LibrarySource, error) {
	d.mu.Lock()
	result := make([]domain.LibrarySource, 0, len(d.accountMounts))
	for _, record := range d.accountMounts {
		if record.ID == "" || record.AccountID == "" {
			continue
		}
		result = append(result, record.source())
	}
	d.mu.Unlock()
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
	d.syncAccountMounts(store)
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
	d.syncAccountMounts(store)
	for id, record := range store {
		if tokensNeedRefresh(record.Tokens) {
			if err := d.refreshAccountTokens(ctx, id); err != nil {
				slog.Warn("refresh 115 account tokens", "account", id, "error", err)
			}
		}
	}
}
