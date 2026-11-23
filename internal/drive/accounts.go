package drive

import (
	"context"
	"log/slog"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/panaccount"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	accountRefreshTick   = 10 * time.Minute
	accountRefreshWindow = 30 * time.Minute
)

// accountRecord is one 115 account's tokens and mounted directory. Each account
// is its own PanAccount row, so writes update a single row and need no global
// lock or in-memory mirror.
type accountRecord struct {
	Tokens    pan.Tokens
	Directory mountRecord
}

func recordOf(row *ent.PanAccount) accountRecord {
	return accountRecord{
		Tokens: pan.Tokens{AccessToken: row.AccessToken, RefreshToken: row.RefreshToken, ExpiresAt: row.ExpiresAt},
		Directory: mountRecord{
			AccountID:        row.AccountID,
			LibraryDirectory: domain.LibraryDirectory{ID: row.DirectoryID, Name: row.DirectoryName, Path: row.DirectoryPath},
		},
	}
}

// accountRecord reads one account. A missing account is an empty record.
func (d *Drive) accountRecord(ctx context.Context, accountID string) (accountRecord, error) {
	row, err := d.database.PanAccount.Query().Where(panaccount.AccountID(accountID)).Only(ctx)
	if ent.IsNotFound(err) {
		return accountRecord{}, nil
	}
	if err != nil {
		return accountRecord{}, err
	}
	return recordOf(row), nil
}

func (d *Drive) accountToken(ctx context.Context, accountID string) (pan.Tokens, error) {
	record, err := d.accountRecord(ctx, accountID)
	return record.Tokens, err
}

func (d *Drive) listAccounts(ctx context.Context) ([]*ent.PanAccount, error) {
	return d.database.PanAccount.Query().Order(ent.Asc(panaccount.FieldAccountID)).All(ctx)
}

// saveAccountTokens stores the tokens of a newly authorized account, creating
// its row when needed. Only login may create accounts.
func (d *Drive) saveAccountTokens(ctx context.Context, accountID string, tokens pan.Tokens) error {
	if accountID == "" {
		return nil
	}
	return d.database.PanAccount.Create().
		SetAccountID(accountID).
		SetAccessToken(tokens.AccessToken).
		SetRefreshToken(tokens.RefreshToken).
		SetExpiresAt(tokens.ExpiresAt).
		OnConflictColumns(panaccount.FieldAccountID).
		Update(func(u *ent.PanAccountUpsert) {
			u.UpdateAccessToken()
			u.UpdateRefreshToken()
			u.UpdateExpiresAt()
			u.UpdateUpdatedAt()
		}).
		Exec(ctx)
}

// updateAccountTokens stores refreshed tokens of an existing account. It never
// recreates a row, so a refresh that finishes after the account was
// disconnected reports ErrUnauthorized instead of resurrecting it.
func (d *Drive) updateAccountTokens(ctx context.Context, accountID string, tokens pan.Tokens) error {
	updated, err := d.database.PanAccount.Update().
		Where(panaccount.AccountID(accountID)).
		SetAccessToken(tokens.AccessToken).
		SetRefreshToken(tokens.RefreshToken).
		SetExpiresAt(tokens.ExpiresAt).
		Save(ctx)
	if err == nil && updated == 0 {
		return pan.ErrUnauthorized
	}
	return err
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
	d.accountCredVersions[accountID]++
}

// bumpAccountAuthorization invalidates reads tied to an older mount or login.
func (d *Drive) bumpAccountAuthorization(accountID string) {
	if accountID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.accountAuthVersions[accountID]++
}

// saveAccountMount stores the mounted directory of an existing account; an
// empty directory clears the mount. An account removed meanwhile reports
// ErrUnauthorized, so callers do not publish a mount for it.
func (d *Drive) saveAccountMount(ctx context.Context, accountID string, mount mountRecord) error {
	if accountID == "" {
		return nil
	}
	d.bumpAccountAuthorization(accountID)
	updated, err := d.database.PanAccount.Update().
		Where(panaccount.AccountID(accountID)).
		SetDirectoryID(mount.ID).
		SetDirectoryName(mount.Name).
		SetDirectoryPath(mount.Path).
		Save(ctx)
	if err == nil && updated == 0 {
		return pan.ErrUnauthorized
	}
	return err
}

func (d *Drive) removeAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	removed, err := d.database.PanAccount.Delete().Where(panaccount.AccountID(accountID)).Exec(ctx)
	if err != nil || removed == 0 {
		return err
	}
	d.bumpAccountCredential(accountID)
	d.bumpAccountAuthorization(accountID)
	return nil
}

// Mount returns the stored library source of an account when it has one. A
// missing account or mount reports ok=false with a nil error.
func (d *Drive) Mount(ctx context.Context, accountID string) (domain.LibrarySource, bool, error) {
	record, err := d.accountRecord(ctx, accountID)
	if err != nil {
		return domain.LibrarySource{}, false, err
	}
	if record.Directory.ID == "" {
		return domain.LibrarySource{}, false, nil
	}
	return record.Directory.source(), true, nil
}

// Mounts returns every stored account's mounted library source, without
// verifying credentials, so callers can operate each account independently.
func (d *Drive) Mounts(ctx context.Context) ([]domain.LibrarySource, error) {
	rows, err := d.database.PanAccount.Query().
		Where(panaccount.DirectoryIDNEQ("")).
		Order(ent.Asc(panaccount.FieldAccountID), ent.Asc(panaccount.FieldDirectoryID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.LibrarySource, 0, len(rows))
	for _, row := range rows {
		result = append(result, recordOf(row).Directory.source())
	}
	return result, nil
}

// Accounts lists every stored 115 account and its mounted directory.
func (d *Drive) Accounts(ctx context.Context) ([]AccountStatus, error) {
	rows, err := d.listAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AccountStatus, 0, len(rows))
	for _, row := range rows {
		id := row.AccountID
		record := recordOf(row)
		if tokensNeedRefresh(record.Tokens) {
			if err := d.refreshAccountTokens(ctx, id); err != nil {
				slog.Warn("refresh 115 account tokens", "account", id, "error", err)
				result = append(result, AccountStatus{AccountID: id, Connected: false})
				continue
			}
			if record.Tokens, err = d.accountToken(ctx, id); err != nil {
				return nil, err
			}
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
	rows, err := d.listAccounts(ctx)
	if err != nil {
		slog.Warn("load 115 accounts for refresh", "error", err)
		return
	}
	for _, row := range rows {
		if tokensNeedRefresh(recordOf(row).Tokens) {
			if err := d.refreshAccountTokens(ctx, row.AccountID); err != nil {
				slog.Warn("refresh 115 account tokens", "account", row.AccountID, "error", err)
			}
		}
	}
}
