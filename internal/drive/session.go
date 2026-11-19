package drive

import (
	"context"
	"slices"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

// Session represents controlled access to one 115 account during an operation.
type Session interface {
	Source() domain.LibrarySource
	// AuthorizationVersion changes on login or mount changes for this account.
	AuthorizationVersion() uint64
	List(ctx context.Context, dirID string, offset int) (pan.FilePage, error)
	Info(ctx context.Context, fileID string) (pan.FileInfo, error)
	Read(ctx context.Context, pickCode string, limit int64) ([]byte, error)
	WithSource(ctx context.Context, fn func() error) error
	Commit(ctx context.Context, fn func(tx *ent.Tx) error) error
	CommitAccount(ctx context.Context, fn func(tx *ent.Tx) error) error

	PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error)
	DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error)
	AddOffline(ctx context.Context, magnet string) (string, error)
	RemoveOffline(ctx context.Context, hash string) error
	OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error)
}

// OpenSource opens a Session for the expected source using the account registry
// for every account; no account is privileged.
func (d *Drive) OpenSource(ctx context.Context, expected domain.LibrarySource) (Session, error) {
	if d.closedState() {
		return nil, context.Canceled
	}
	if expected.AccountID == "" {
		return nil, ErrSourceChanged
	}
	record, err := d.accountRecord(ctx, expected.AccountID)
	if err != nil {
		return nil, err
	}
	if record.Tokens.AccessToken == "" {
		return nil, pan.ErrUnauthorized
	}
	if record.Directory.ID != expected.Directory.ID {
		return nil, ErrSourceChanged
	}
	return &accountSession{
		drive:       d,
		accountID:   expected.AccountID,
		source:      expected,
		credVersion: d.accountCredVersion(expected.AccountID),
		authVersion: d.accountAuthVersion(expected.AccountID),
	}, nil
}

// SourceInfo fetches file info and validates that it resides within the session's library source.
func SourceInfo(ctx context.Context, sess Session, id string) (pan.FileInfo, error) {
	info, err := sess.Info(ctx, id)
	if err != nil {
		return pan.FileInfo{}, err
	}
	if !WithinSource(info, sess.Source()) {
		return pan.FileInfo{}, domain.E(domain.KindNotFound, "下载资源已移出媒体目录", nil)
	}
	return info, nil
}

// DirectoryEntries lists all file entries across pages in a directory, validating that the directory is within the library source.
func DirectoryEntries(ctx context.Context, sess Session, id string) ([]pan.File, error) {
	var files []pan.File
	err := WalkFilePages(ctx, func(offset int) (pan.FilePage, error) {
		page, err := sess.List(ctx, id, offset)
		if err != nil {
			return pan.FilePage{}, err
		}
		if !slices.ContainsFunc(page.Path, func(directory pan.Directory) bool { return directory.ID == sess.Source().Directory.ID }) {
			return pan.FilePage{}, domain.E(domain.KindConflict, "该文件夹已移出媒体目录，请重新扫描", nil)
		}
		return page, nil
	}, func(page pan.FilePage) (bool, error) {
		files = append(files, page.Files...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
