package drive

import (
	"context"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

// accountStatusTTL bounds how long an account profile may be reused. The
// profile comes from a provider call; mount changes invalidate it explicitly.
const accountStatusTTL = 6 * time.Hour

func (d *Drive) accountSessionFor(ctx context.Context, accountID string) (*accountSession, error) {
	record, err := d.accountRecord(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if record.Tokens.AccessToken == "" {
		return nil, pan.ErrUnauthorized
	}
	client, ok := d.providerClient(record.Provider)
	if !ok {
		return nil, domain.E(domain.KindInvalid, "存储账号的来源不受支持", nil)
	}
	return &accountSession{
		drive: d, client: client, provider: record.Provider,
		accountID: accountID, source: record.Directory.source(),
	}, nil
}

// AccountStatusFor reports one stored account's profile and mounted directory.
func (d *Drive) AccountStatusFor(ctx context.Context, accountID string) (AccountStatus, error) {
	if status, ok := d.cachedAccountStatus(accountID); ok {
		return status, nil
	}
	generation := d.accountStatusCacheGeneration()
	session, err := d.accountSessionFor(ctx, accountID)
	if err != nil {
		return AccountStatus{}, err
	}
	account, err := accountWithToken(ctx, session, func(token string) (pan.Account, error) {
		return session.client.Account(ctx, token)
	})
	if err != nil {
		return AccountStatus{}, fmt.Errorf("get 115 account: %w", err)
	}
	status := AccountStatus{AccountID: account.ID, Provider: session.provider, Connected: true, Account: &account}
	if record, err := d.accountRecord(ctx, accountID); err == nil && record.Directory.ID != "" && record.Directory.AccountID == account.ID {
		directory := record.Directory.LibraryDirectory
		status.Directory = &directory
	}
	d.storeAccountStatus(accountID, status, generation)
	return status, nil
}

func (d *Drive) cachedAccountStatus(accountID string) (AccountStatus, bool) {
	d.statusMu.Lock()
	defer d.statusMu.Unlock()
	entry, ok := d.accountStatuses[accountID]
	if !ok || d.now().Sub(entry.cachedAt) >= accountStatusTTL {
		return AccountStatus{}, false
	}
	return entry.status, true
}

func (d *Drive) accountStatusCacheGeneration() uint64 {
	d.statusMu.Lock()
	defer d.statusMu.Unlock()
	return d.statusGeneration
}

func (d *Drive) storeAccountStatus(accountID string, status AccountStatus, generation uint64) {
	d.statusMu.Lock()
	// Do not restore a profile read before a concurrent mount or account change.
	if d.statusGeneration == generation {
		d.accountStatuses[accountID] = accountStatusCacheEntry{status: status, cachedAt: d.now()}
	}
	d.statusMu.Unlock()
}

func (d *Drive) invalidateAccountStatus(accountID string) {
	d.statusMu.Lock()
	d.statusGeneration++
	delete(d.accountStatuses, accountID)
	d.statusMu.Unlock()
}

// FilesFor lists a directory of a stored account.
func (d *Drive) FilesFor(ctx context.Context, accountID, directoryID string, page int) (pan.FilePage, error) {
	session, err := d.accountSessionFor(ctx, accountID)
	if err != nil {
		return pan.FilePage{}, err
	}
	return accountWithToken(ctx, session, func(token string) (pan.FilePage, error) {
		return session.client.List(ctx, token, directoryID, (page-1)*100, 100)
	})
}

// SelectDirectoryForAccount mounts a media directory for a stored account.
func (d *Drive) SelectDirectoryForAccount(ctx context.Context, accountID, directoryID string) (domain.LibraryDirectory, error) {
	session, err := d.accountSessionFor(ctx, accountID)
	if err != nil {
		return domain.LibraryDirectory{}, err
	}
	files, err := accountWithToken(ctx, session, func(token string) (pan.FilePage, error) {
		return session.client.List(ctx, token, directoryID, 0, 1)
	})
	if err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("get media directory: %w", err)
	}
	mountProvider := session.provider
	if mountProvider == Provider115 {
		mountProvider = ""
	}
	var directory domain.LibraryDirectory
	if directoryID == "0" && session.provider == ProviderGuangya {
		directory = domain.LibraryDirectory{ID: "0", Name: "光鸭根目录", Path: "/"}
	} else {
		if len(files.Path) == 0 {
			return domain.LibraryDirectory{}, domain.E(domain.KindNotFound, "目录不存在或已被移除", nil)
		}
		directory = domain.LibraryDirectory{
			ID:   directoryID,
			Name: files.Path[len(files.Path)-1].Name,
			Path: directoryPath(files.Path),
		}
	}
	record := mountRecord{AccountID: accountID, Provider: mountProvider, LibraryDirectory: directory}
	if err := d.saveAccountMount(ctx, accountID, record); err != nil {
		return domain.LibraryDirectory{}, err
	}
	// The library queues its scan from this event; a stored mount alone is silent.
	// The mount above is already persisted, so a failure here is surfaced to the
	// caller without rolling that change back.
	if err := d.publishMount(ctx, domain.LibrarySource{Provider: mountProvider, AccountID: accountID, Directory: directory}); err != nil {
		return domain.LibraryDirectory{}, err
	}
	return directory, nil
}

// ClearDirectoryForAccount unmounts a stored account's media directory.
func (d *Drive) ClearDirectoryForAccount(ctx context.Context, accountID string) error {
	return d.saveAccountMount(ctx, accountID, mountRecord{AccountID: accountID})
}

// DisconnectAccount removes a stored account and its mounted directory.
func (d *Drive) DisconnectAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	return d.removeAccount(ctx, accountID)
}
