package drive

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

func (d *Drive) accountSessionFor(ctx context.Context, accountID string) (*accountSession, error) {
	record, err := d.accountRecord(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if record.Tokens.AccessToken == "" {
		return nil, pan.ErrUnauthorized
	}
	return &accountSession{drive: d, accountID: accountID, source: record.Directory.source()}, nil
}

// AccountStatusFor reports one stored account's profile and mounted directory.
func (d *Drive) AccountStatusFor(ctx context.Context, accountID string) (AccountStatus, error) {
	session, err := d.accountSessionFor(ctx, accountID)
	if err != nil {
		return AccountStatus{}, err
	}
	account, err := accountWithToken(ctx, session, func(token string) (pan.Account, error) {
		return d.client.Account(ctx, token)
	})
	if err != nil {
		return AccountStatus{}, fmt.Errorf("get 115 account: %w", err)
	}
	status := AccountStatus{AccountID: account.ID, Connected: true, Account: &account}
	if record, err := d.accountRecord(ctx, accountID); err == nil && record.Directory.ID != "" && record.Directory.AccountID == account.ID {
		directory := record.Directory.LibraryDirectory
		status.Directory = &directory
	}
	return status, nil
}

// FilesFor lists a directory of a stored account.
func (d *Drive) FilesFor(ctx context.Context, accountID, directoryID string, page int) (pan.FilePage, error) {
	session, err := d.accountSessionFor(ctx, accountID)
	if err != nil {
		return pan.FilePage{}, err
	}
	return accountWithToken(ctx, session, func(token string) (pan.FilePage, error) {
		return d.client.List(ctx, token, directoryID, (page-1)*100, 100)
	})
}

// SelectDirectoryForAccount mounts a media directory for a stored account.
func (d *Drive) SelectDirectoryForAccount(ctx context.Context, accountID, directoryID string) (domain.LibraryDirectory, error) {
	session, err := d.accountSessionFor(ctx, accountID)
	if err != nil {
		return domain.LibraryDirectory{}, err
	}
	files, err := accountWithToken(ctx, session, func(token string) (pan.FilePage, error) {
		return d.client.List(ctx, token, directoryID, 0, 1)
	})
	if err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("get 115 media directory: %w", err)
	}
	if len(files.Path) == 0 {
		return domain.LibraryDirectory{}, domain.E(domain.KindNotFound, "115 目录不存在或已被移除", nil)
	}
	directory := domain.LibraryDirectory{
		ID:   directoryID,
		Name: files.Path[len(files.Path)-1].Name,
		Path: directoryPath(files.Path),
	}
	record := mountRecord{AccountID: accountID, LibraryDirectory: directory}
	if err := d.saveAccountMount(ctx, accountID, record); err != nil {
		return domain.LibraryDirectory{}, err
	}
	// The library queues its scan from this event; a stored mount alone is silent.
	// The mount above is already persisted, so a failure here is surfaced to the
	// caller without rolling that change back.
	if err := d.publishMount(ctx, domain.LibrarySource{AccountID: accountID, Directory: directory}); err != nil {
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
