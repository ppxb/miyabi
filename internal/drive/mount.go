package drive

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/pan"
)

// Files retrieves a single page of directory items from 115 for folder navigation.
func (d *Drive) Files(ctx context.Context, directoryID string, page int) (pan.FilePage, error) {
	state := d.snapshot()
	files, err := withPanToken(ctx, d, state, func(token string) (pan.FilePage, error) {
		return d.client.List(ctx, token, directoryID, (page-1)*100, 100)
	})
	if err != nil {
		return pan.FilePage{}, fmt.Errorf("list 115 directory: %w", err)
	}
	if _, err := d.credentials(state); err != nil {
		return pan.FilePage{}, err
	}
	return files, nil
}

// SelectDirectory mounts the given 115 directory as the media root, saves it,
// updates the in-memory state, and emits a MountChanged event.
func (d *Drive) SelectDirectory(ctx context.Context, directoryID string) (domain.LibraryDirectory, error) {
	state := d.snapshot()
	account, err := d.verifyAccount(ctx, state)
	if err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("get 115 account for directory: %w", err)
	}
	files, err := withPanToken(ctx, d, state, func(token string) (pan.FilePage, error) {
		return d.client.List(ctx, token, directoryID, 0, 1)
	})
	if err != nil {
		return domain.LibraryDirectory{}, fmt.Errorf("get 115 media directory: %w", err)
	}
	if len(files.Path) == 0 {
		return domain.LibraryDirectory{}, domain.E(domain.KindNotFound, "115 目录不存在或已被移除", nil)
	}
	directory := domain.LibraryDirectory{
		ID: directoryID, Name: files.Path[len(files.Path)-1].Name,
		Path: directoryPath(files.Path),
	}
	record := mountRecord{AccountID: account.ID, LibraryDirectory: directory}

	if err := d.commit.Lock(ctx); err != nil {
		return domain.LibraryDirectory{}, err
	}
	defer d.commit.Unlock()

	current, err := d.credentials(state)
	if err != nil {
		return domain.LibraryDirectory{}, err
	}
	if current.directory.AccountID == record.AccountID && current.directory.ID == record.ID {
		return current.directory.LibraryDirectory, nil
	}
	if _, err := d.sourceState(state.source(), state.authorizationVersion); err != nil {
		return domain.LibraryDirectory{}, err
	}

	if err := d.persistDirectory(ctx, record); err != nil {
		return domain.LibraryDirectory{}, err
	}
	previous := d.swapDirectory(record)
	// The mount is complete only once the listener has accepted it; the
	// library queues the scan here. On failure nothing observable remains.
	if err := d.publishMount(ctx, record.source()); err != nil {
		d.restoreDirectory(previous)
		if rollback := d.persistDirectory(ctx, previous.directory); rollback != nil {
			return domain.LibraryDirectory{}, fmt.Errorf("mount media directory: %w (rollback failed: %v)", err, rollback)
		}
		return domain.LibraryDirectory{}, fmt.Errorf("mount media directory: %w", err)
	}
	return directory, nil
}

// swapDirectory publishes a new mount in memory and returns what it replaced.
func (d *Drive) swapDirectory(record mountRecord) snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	previous := snapshot{directory: d.directory, authorizationVersion: d.authorizationVersion}
	d.directory = record
	d.authorizationVersion++
	return previous
}

func (d *Drive) restoreDirectory(previous snapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.directory = previous.directory
	d.authorizationVersion = previous.authorizationVersion
}

// persistDirectory stores the mount, or removes it when the record is empty.
func (d *Drive) persistDirectory(ctx context.Context, record mountRecord) error {
	if record.ID == "" {
		if _, err := d.database.Setting.Delete().Where(setting.Key(directorySetting)).Exec(ctx); err != nil {
			return fmt.Errorf("remove 115 media directory setting: %w", err)
		}
		return nil
	}
	if err := database.SaveSetting(ctx, d.database, directorySetting, record); err != nil {
		return fmt.Errorf("save media directory setting: %w", err)
	}
	return nil
}

// ClearDirectory unmounts the active media directory and emits a MountChanged event.
func (d *Drive) ClearDirectory(ctx context.Context) error {
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	return d.clearDirectory(ctx)
}

func (d *Drive) discardOtherAccountDirectory(ctx context.Context, accountID string) error {
	directory := d.snapshot().directory
	if directory.ID == "" || directory.AccountID == accountID {
		return nil
	}
	return d.clearDirectory(ctx)
}

func (d *Drive) clearDirectory(ctx context.Context) error {
	if err := d.persistDirectory(ctx, mountRecord{}); err != nil {
		return err
	}
	d.swapDirectory(mountRecord{})
	if err := d.publishMount(ctx, domain.LibrarySource{}); err != nil {
		return fmt.Errorf("publish unmount event: %w", err)
	}
	return nil
}

// WithinSource checks whether a file info record belongs inside the mounted library source.
func WithinSource(info pan.FileInfo, source domain.LibrarySource) bool {
	return info.ID == source.Directory.ID || source.Directory.ID == "0" ||
		slices.ContainsFunc(info.Path, func(dir pan.Directory) bool { return dir.ID == source.Directory.ID })
}

// directoryPath builds a root-anchored path string from 115 path segments,
// skipping the root folder (ID "0").
func directoryPath(segments []pan.Directory) string {
	names := make([]string, 0, len(segments))
	for _, directory := range segments {
		if directory.ID != "0" && directory.Name != "" {
			names = append(names, directory.Name)
		}
	}
	return "/" + strings.Join(names, "/")
}

// FilePath builds a root-anchored path string from 115 path segments and a file name.
func FilePath(segments []pan.Directory, name string) string {
	dir := directoryPath(segments)
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}
