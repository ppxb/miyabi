package drive

import (
	"context"
	"slices"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

// Session represents controlled access to 115 during an operation.
// It captures account, mounted directory, and authorization version at issue;
// checks versions before and after each request; and serializes database
// commits under the drive commit lock.
type Session interface {
	Source() domain.LibrarySource
	Version() uint64
	List(ctx context.Context, dirID string, offset int) (pan.FilePage, error)
	Info(ctx context.Context, fileID string) (pan.FileInfo, error)
	Read(ctx context.Context, pickCode string, limit int64) ([]byte, error)
	Upload(ctx context.Context, dirID, name string, body []byte) error
	Commit(ctx context.Context, fn func(tx *ent.Tx) error) error
	CommitAccount(ctx context.Context, fn func(tx *ent.Tx) error) error

	PlayURL(ctx context.Context, pickCode string) ([]pan.PlaySource, error)
	DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error)
	AddOffline(ctx context.Context, magnet string) (string, error)
	RemoveOffline(ctx context.Context, hash string) error
	OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error)
}

// Open verifies the current mount and authenticated account, returning a Session.
func (d *Drive) Open(ctx context.Context) (Session, error) {
	state, err := d.verifiedSource(ctx)
	if err != nil {
		return nil, err
	}
	return &sourceSession{
		drive:             d,
		source:            state.source(),
		version:           state.authorizationVersion,
		credentialVersion: state.credentialVersion,
	}, nil
}

// OpenSource opens a Session ensuring the current mount matches the expected source.
func (d *Drive) OpenSource(ctx context.Context, expected domain.LibrarySource) (Session, error) {
	sess, err := d.Open(ctx)
	if err != nil {
		return nil, err
	}
	if sess.Source().AccountID != expected.AccountID || sess.Source().Directory.ID != expected.Directory.ID {
		return nil, ErrSourceChanged
	}
	return sess, nil
}

type sourceSession struct {
	drive             *Drive
	source            domain.LibrarySource
	version           uint64
	credentialVersion uint64
}

func (s *sourceSession) Source() domain.LibrarySource {
	return s.source
}

func (s *sourceSession) Version() uint64 {
	return s.version
}

func (s *sourceSession) checkSource() error {
	_, err := s.drive.sourceState(s.source, s.version)
	return err
}

func (s *sourceSession) List(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return pan.FilePage{}, err
	}
	page, err := withPanSourceToken(ctx, s.drive, state, func(token string) (pan.FilePage, error) {
		return s.drive.client.List(ctx, token, dirID, offset, 100)
	})
	if err != nil {
		return pan.FilePage{}, err
	}
	if err := s.checkSource(); err != nil {
		return pan.FilePage{}, err
	}
	return page, nil
}

func (s *sourceSession) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return pan.FileInfo{}, err
	}
	info, err := withPanSourceToken(ctx, s.drive, state, func(token string) (pan.FileInfo, error) {
		return s.drive.client.Info(ctx, token, fileID)
	})
	if err != nil {
		return pan.FileInfo{}, err
	}
	if err := s.checkSource(); err != nil {
		return pan.FileInfo{}, err
	}
	return info, nil
}

func (s *sourceSession) Read(ctx context.Context, pickCode string, limit int64) ([]byte, error) {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return nil, err
	}
	body, err := withPanSourceToken(ctx, s.drive, state, func(token string) ([]byte, error) {
		return s.drive.client.ReadMetadata(ctx, token, pickCode, limit)
	})
	if err != nil {
		return nil, err
	}
	if err := s.checkSource(); err != nil {
		return nil, err
	}
	return body, nil
}

func (s *sourceSession) Upload(ctx context.Context, dirID, name string, body []byte) error {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return err
	}
	_, err = withPanSourceToken(ctx, s.drive, state, func(token string) (struct{}, error) {
		return struct{}{}, s.drive.client.UploadMetadata(ctx, token, dirID, name, body)
	})
	if err != nil {
		return err
	}
	return s.checkSource()
}

func (s *sourceSession) PlayURL(ctx context.Context, pickCode string) ([]pan.PlaySource, error) {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return nil, err
	}
	sources, err := withPanSourceToken(ctx, s.drive, state, func(token string) ([]pan.PlaySource, error) {
		return s.drive.client.PlayURL(ctx, token, pickCode)
	})
	if err != nil {
		return nil, err
	}
	if err := s.checkSource(); err != nil {
		return nil, err
	}
	return sources, nil
}

func (s *sourceSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return "", err
	}
	downloadURL, err := withPanSourceToken(ctx, s.drive, state, func(token string) (string, error) {
		return s.drive.client.DownloadURL(ctx, token, pickCode, userAgent)
	})
	if err != nil {
		return "", err
	}
	if err := s.checkSource(); err != nil {
		return "", err
	}
	return downloadURL, nil
}

func (s *sourceSession) AddOffline(ctx context.Context, magnet string) (string, error) {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return "", err
	}
	return withPanSourceToken(ctx, s.drive, state, func(token string) (string, error) {
		return s.drive.client.AddOffline(ctx, token, magnet, s.source.Directory.ID)
	})
}

func (s *sourceSession) RemoveOffline(ctx context.Context, hash string) error {
	state, err := s.drive.sourceState(s.source, s.version)
	if err != nil {
		return err
	}
	_, err = withPanSourceToken(ctx, s.drive, state, func(token string) (struct{}, error) {
		return struct{}{}, s.drive.client.RemoveOffline(ctx, token, hash)
	})
	if err != nil {
		return err
	}
	return s.checkSource()
}

func (s *sourceSession) OfflineTasks(ctx context.Context, page int) (pan.OfflinePage, error) {
	state := snapshot{credentialVersion: s.credentialVersion}
	return withPanToken(ctx, s.drive, state, func(token string) (pan.OfflinePage, error) {
		return s.drive.client.OfflineTasks(ctx, token, page)
	})
}

func (s *sourceSession) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	if err := s.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer s.drive.commit.Unlock()
	if _, err := s.drive.sourceState(s.source, s.version); err != nil {
		return err
	}
	return ent.WithTx(ctx, s.drive.database, fn)
}

func (s *sourceSession) CommitAccount(ctx context.Context, fn func(tx *ent.Tx) error) error {
	if err := s.drive.commit.Lock(ctx); err != nil {
		return err
	}
	defer s.drive.commit.Unlock()
	current, err := s.drive.credentials(snapshot{credentialVersion: s.credentialVersion})
	if err != nil {
		return err
	}
	if current.closed {
		return context.Canceled
	}
	return ent.WithTx(ctx, s.drive.database, fn)
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
