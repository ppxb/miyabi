package drive

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

func mountFixture(t *testing.T) (*Drive, *stubClient) {
	t.Helper()
	client := &stubClient{}
	d := newTestDrive(t, client)
	loginTestAccount(t, d)
	return d, client
}

// mountRecorder observes mount events and can reject a mount.
type mountRecorder struct {
	mu      sync.Mutex
	calls   int
	last    MountEvent
	failure error
}

func recordMounts(d *Drive) *mountRecorder {
	r := &mountRecorder{}
	d.SetMountListener(func(ctx context.Context, event MountEvent) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls++
		r.last = event
		return r.failure
	})
	return r
}

func (r *mountRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *mountRecorder) lastEvent() MountEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func (r *mountRecorder) reject(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failure = err
}

func TestMountListenerReplacementAndClearing(t *testing.T) {
	d, _ := mountFixture(t)
	d.SetMountListener(func(context.Context, MountEvent) error {
		return errors.New("replaced listener must not run")
	})
	var calls atomic.Int32
	d.SetMountListener(func(_ context.Context, event MountEvent) error {
		calls.Add(1)
		// Reading state and clearing the callback must not deadlock on d.mu.
		source := d.Source()
		if source == nil || *source != event.Source {
			return errors.New("listener did not observe the new mount")
		}
		d.SetMountListener(nil)
		return nil
	})
	finished := make(chan error, 1)
	go func() {
		_, err := d.SelectDirectory(t.Context(), "20")
		finished <- err
	}()
	if err := await(t, finished); err != nil {
		t.Fatal(err)
	}
	if err := d.ClearDirectory(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || d.Source() != nil {
		t.Fatalf("cleared listener: calls=%d source=%+v", calls.Load(), d.Source())
	}
}

func TestDirectoryMountPublishesOnceAndDoesNotInterruptTheSameMount(t *testing.T) {
	d, _ := mountFixture(t)
	ctx := t.Context()
	mounts := recordMounts(d)
	directory, err := d.SelectDirectory(ctx, "20")
	if err != nil || directory.ID != "20" || directory.Path != "/Movies20" {
		t.Fatalf("mount: %+v err=%v", directory, err)
	}
	event := mounts.lastEvent()
	if mounts.count() != 1 || event.Source.AccountID != testSource.AccountID || event.Source.Directory != directory {
		t.Fatalf("mount did not publish its source once: calls=%d event=%+v", mounts.count(), event)
	}
	version := d.snapshot().authorizationVersion
	for range 3 {
		if _, err := d.SelectDirectory(ctx, "20"); err != nil {
			t.Fatal(err)
		}
	}
	if d.snapshot().authorizationVersion != version || mounts.count() != 1 {
		t.Fatal("repeated mount interrupted active work or published again")
	}
	if _, err := d.SelectDirectory(ctx, "30"); err != nil {
		t.Fatal(err)
	}
	if d.snapshot().authorizationVersion != version+1 || mounts.count() != 2 {
		t.Fatal("changing directory did not publish exactly one new mount")
	}
	// Startup restores the durable mount without publishing.
	restarted, err := NewWithClient(ctx, d.database, d.client)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if source := restarted.Source(); source == nil || source.Directory.ID != "30" || source.AccountID != testSource.AccountID {
		t.Fatalf("restart lost the mount: %+v", source)
	}
}

func TestMountRollsBackWhenAListenerRejectsIt(t *testing.T) {
	d, client := mountFixture(t)
	ctx := t.Context()
	selectTestDirectory(t, d, client, testSource.Directory)
	before := d.snapshot()
	mounts := recordMounts(d)
	rejected := errors.New("cannot queue scan")
	mounts.reject(rejected)
	if _, err := d.SelectDirectory(ctx, "20"); !errors.Is(err, rejected) {
		t.Fatalf("listener failure=%v", err)
	}
	after := d.snapshot()
	saved, found := savedDirectory(t, d)
	if mounts.count() != 1 || after.directory != before.directory || after.authorizationVersion != before.authorizationVersion ||
		!found || saved != before.directory {
		t.Fatalf("failed mount left partial state: memory=%+v saved=%+v found=%t", after.directory, saved, found)
	}
	mounts.reject(nil)
	if _, err := d.SelectDirectory(ctx, "20"); err != nil || d.Source().Directory.ID != "20" {
		t.Fatalf("retry after a rejected mount: %v", err)
	}
	if sess, err := d.Open(ctx); err != nil || sess.Source().Directory.ID != "20" {
		t.Fatalf("session after retry = %v, %v", sess, err)
	}
}

func TestLateMountRequestsCannotReplaceANewerSource(t *testing.T) {
	for _, next := range []string{"20", "30", "disconnect"} {
		t.Run(next, func(t *testing.T) {
			d, client := mountFixture(t)
			ctx := t.Context()
			started := make(chan struct{}, 1)
			hold, release := testGate(t)
			var listings atomic.Int32
			client.list = func(_ context.Context, _, id string, _, _ int) (pan.FilePage, error) {
				if listings.Add(1) == 1 {
					started <- struct{}{}
					<-hold
				}
				return directoryPage(domain.LibraryDirectory{ID: id, Name: "Movies" + id, Path: "/Movies" + id}), nil
			}
			mounts := recordMounts(d)
			finished := make(chan error, 1)
			go func() { _, err := d.SelectDirectory(ctx, "20"); finished <- err }()
			await(t, started)
			if next == "disconnect" {
				if _, err := d.Disconnect(ctx); err != nil {
					t.Fatal(err)
				}
			} else if _, err := d.SelectDirectory(ctx, next); err != nil {
				t.Fatal(err)
			}
			version := d.snapshot().authorizationVersion
			published := mounts.count()
			release()
			err := await(t, finished)
			wantID := next
			switch next {
			case "20":
				if err != nil {
					t.Fatalf("same mount retry failed: %v", err)
				}
			case "30":
				if !errors.Is(err, ErrSourceChanged) {
					t.Fatalf("stale directory request=%v", err)
				}
			case "disconnect":
				wantID = ""
				if !errors.Is(err, pan.ErrUnauthorized) {
					t.Fatalf("stale account request=%v", err)
				}
			}
			if d.snapshot().directory.ID != wantID || d.snapshot().authorizationVersion != version || mounts.count() != published {
				t.Fatal("late request changed the source or published another mount")
			}
		})
	}
}

func TestVerifiedAccountKeepsOnlyItsOwnDirectory(t *testing.T) {
	d, client := mountedTestDrive(t)
	ctx := t.Context()
	mounts := recordMounts(d)
	if err := d.discardOtherAccountDirectory(ctx, testSource.AccountID); err != nil {
		t.Fatal(err)
	}
	if source := d.Source(); source == nil || source.Directory.ID != testSource.Directory.ID || mounts.count() != 0 {
		t.Fatalf("same-account verification touched the mount: source=%v calls=%d", source, mounts.count())
	}
	client.account = func(context.Context, string) (pan.Account, error) { return pan.Account{ID: "different-account"}, nil }
	d.invalidateAccountCache()
	status, err := d.Account(ctx)
	if err != nil || !status.Connected || status.Directory != nil {
		t.Fatalf("other account status = %+v, %v", status, err)
	}
	if _, found := savedDirectory(t, d); found || d.Source() != nil || mounts.count() != 1 || mounts.lastEvent().Source.Directory.ID != "" {
		t.Fatalf("old account selection retained: source=%v calls=%d", d.Source(), mounts.count())
	}
}

func TestClearingAnUnmountedDriveIsHarmless(t *testing.T) {
	d, _ := mountFixture(t)
	for range 2 {
		if err := d.ClearDirectory(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if d.Source() != nil {
		t.Fatalf("source after clearing = %+v", d.Source())
	}
	if _, found := savedDirectory(t, d); found {
		t.Fatal("a directory setting appeared while clearing")
	}
}

func TestSessionsTrackTheMountTheyWereIssuedAgainst(t *testing.T) {
	d, client := mountedTestDrive(t)
	ctx := t.Context()
	sess, err := d.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Source() != testSource {
		t.Fatalf("session source = %+v", sess.Source())
	}
	if _, err := d.OpenSource(ctx, domain.LibrarySource{AccountID: testSource.AccountID, Directory: domain.LibraryDirectory{ID: "other"}}); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("session for another source = %v", err)
	}
	client.list = func(_ context.Context, _, id string, _, _ int) (pan.FilePage, error) {
		return pan.FilePage{Total: 1, Files: []pan.File{{ID: "f1"}}, Path: []pan.Directory{{ID: "0"}, {ID: id}}}, nil
	}
	if page, err := sess.List(ctx, testSource.Directory.ID, 0); err != nil || len(page.Files) != 1 {
		t.Fatalf("list = %+v, %v", page, err)
	}
	if info, err := sess.Info(ctx, "f1"); err != nil || info.ID != "f1" {
		t.Fatalf("info = %+v, %v", info, err)
	}
	if body, err := sess.Read(ctx, "pick-f1", 1024); err != nil || len(body) == 0 {
		t.Fatalf("read = %q, %v", body, err)
	}
	committed := false
	if err := sess.Commit(ctx, func(*ent.Tx) error { committed = true; return nil }); err != nil || !committed {
		t.Fatalf("commit ran=%t err=%v", committed, err)
	}
	selectTestDirectory(t, d, client, domain.LibraryDirectory{ID: "20", Name: "Other", Path: "/Other"})
	for name, call := range map[string]func() error{
		"list":   func() error { _, err := sess.List(ctx, testSource.Directory.ID, 0); return err },
		"info":   func() error { _, err := sess.Info(ctx, "f1"); return err },
		"read":   func() error { _, err := sess.Read(ctx, "pick-f1", 1024); return err },
		"play":   func() error { _, err := sess.PlayURL(ctx, "pick-f1", ""); return err },
		"commit": func() error { return sess.Commit(ctx, func(*ent.Tx) error { return nil }) },
	} {
		if err := call(); !errors.Is(err, ErrSourceChanged) {
			t.Fatalf("%s on a replaced mount = %v", name, err)
		}
	}
	// Account-scoped work outlives the mount but not the credentials.
	if err := sess.CommitAccount(ctx, func(*ent.Tx) error { return nil }); err != nil {
		t.Fatalf("account commit after remount = %v", err)
	}
	if _, err := d.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.CommitAccount(ctx, func(*ent.Tx) error { return nil }); !errors.Is(err, pan.ErrUnauthorized) {
		t.Fatalf("account commit after logout = %v", err)
	}
}

func TestOpenRequiresAMountedDirectoryOfTheVerifiedAccount(t *testing.T) {
	d, client := mountFixture(t)
	ctx := t.Context()
	if _, err := d.Open(ctx); !errors.Is(err, ErrMediaDirectoryRequired) {
		t.Fatalf("open without a mount = %v", err)
	}
	selectTestDirectory(t, d, client, testSource.Directory)
	if _, err := d.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Open(ctx); !errors.Is(err, pan.ErrUnauthorized) {
		t.Fatalf("open after logout = %v", err)
	}
}

func TestWithinSource(t *testing.T) {
	source := testSource
	for name, test := range map[string]struct {
		info pan.FileInfo
		want bool
	}{
		"the root itself": {pan.FileInfo{File: pan.File{ID: source.Directory.ID}}, true},
		"a descendant":    {pan.FileInfo{File: pan.File{ID: "200"}, Path: []pan.Directory{{ID: "0"}, {ID: source.Directory.ID}, {ID: "150"}}}, true},
		"outside":         {pan.FileInfo{File: pan.File{ID: "300"}, Path: []pan.Directory{{ID: "0"}, {ID: "999"}}}, false},
	} {
		if got := WithinSource(test.info, source); got != test.want {
			t.Fatalf("%s: within=%t, want %t", name, got, test.want)
		}
	}
	everything := domain.LibrarySource{Directory: domain.LibraryDirectory{ID: "0"}}
	if !WithinSource(pan.FileInfo{File: pan.File{ID: "300"}}, everything) {
		t.Fatal("the 115 root should contain everything")
	}
}

func TestDirectoryPath(t *testing.T) {
	for name, tt := range map[string]struct {
		segments []pan.Directory
		want     string
	}{
		"empty": {segments: nil, want: "/"},
		"root only": {
			segments: []pan.Directory{{ID: "0", Name: "根目录"}},
			want:     "/",
		},
		"single level": {
			segments: []pan.Directory{{ID: "0", Name: "根目录"}, {ID: "1", Name: "Movies"}},
			want:     "/Movies",
		},
		"multi level": {
			segments: []pan.Directory{{ID: "0", Name: "根目录"}, {ID: "1", Name: "Media"}, {ID: "2", Name: "Jav"}},
			want:     "/Media/Jav",
		},
	} {
		if got := directoryPath(tt.segments); got != tt.want {
			t.Errorf("%s: directoryPath() = %q, want %q", name, got, tt.want)
		}
	}
}

func TestFilePath(t *testing.T) {
	segments := []pan.Directory{{ID: "0", Name: "根目录"}, {ID: "1", Name: "Movies"}}
	if got := FilePath(segments, "SSIS-589.mp4"); got != "/Movies/SSIS-589.mp4" {
		t.Errorf("FilePath() = %q, want %q", got, "/Movies/SSIS-589.mp4")
	}
	if got := FilePath(nil, "root.mp4"); got != "/root.mp4" {
		t.Errorf("FilePath() = %q, want %q", got, "/root.mp4")
	}
}

type fakeSessionStub struct {
	Session
	source domain.LibrarySource
	info   func(ctx context.Context, fileID string) (pan.FileInfo, error)
	list   func(ctx context.Context, dirID string, offset int) (pan.FilePage, error)
}

func (f *fakeSessionStub) Source() domain.LibrarySource { return f.source }
func (f *fakeSessionStub) Info(ctx context.Context, id string) (pan.FileInfo, error) {
	if f.info != nil {
		return f.info(ctx, id)
	}
	return pan.FileInfo{}, nil
}
func (f *fakeSessionStub) List(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
	if f.list != nil {
		return f.list(ctx, dirID, offset)
	}
	return pan.FilePage{}, nil
}

func TestSourceInfo(t *testing.T) {
	source := domain.LibrarySource{
		AccountID: "acc-1",
		Directory: domain.LibraryDirectory{ID: "dir-100"},
	}
	sess := &fakeSessionStub{
		source: source,
		info: func(ctx context.Context, fileID string) (pan.FileInfo, error) {
			if fileID == "valid" {
				return pan.FileInfo{
					File: pan.File{ID: "valid", Name: "ABP-001.mp4"},
					Path: []pan.Directory{{ID: "0"}, {ID: "dir-100"}},
				}, nil
			}
			return pan.FileInfo{
				File: pan.File{ID: "outside", Name: "other.mp4"},
				Path: []pan.Directory{{ID: "0"}, {ID: "other-dir"}},
			}, nil
		},
	}

	info, err := SourceInfo(t.Context(), sess, "valid")
	if err != nil || info.Name != "ABP-001.mp4" {
		t.Fatalf("expected valid info, got err=%v, info=%+v", err, info)
	}

	_, err = SourceInfo(t.Context(), sess, "outside")
	if err == nil {
		t.Fatal("expected error for file outside library source, got nil")
	}
}

func TestDirectoryEntries(t *testing.T) {
	source := domain.LibrarySource{
		AccountID: "acc-1",
		Directory: domain.LibraryDirectory{ID: "dir-100"},
	}
	sess := &fakeSessionStub{
		source: source,
		list: func(ctx context.Context, dirID string, offset int) (pan.FilePage, error) {
			if dirID == "dir-inside" {
				return pan.FilePage{
					Total: 2,
					Path:  []pan.Directory{{ID: "0"}, {ID: "dir-100"}, {ID: "dir-inside"}},
					Files: []pan.File{{ID: "f1", Name: "a.mp4"}, {ID: "f2", Name: "b.mp4"}},
				}, nil
			}
			return pan.FilePage{
				Total: 1,
				Path:  []pan.Directory{{ID: "0"}, {ID: "other"}},
				Files: []pan.File{{ID: "f3", Name: "c.mp4"}},
			}, nil
		},
	}

	files, err := DirectoryEntries(t.Context(), sess, "dir-inside")
	if err != nil || len(files) != 2 {
		t.Fatalf("expected 2 files, got err=%v, files=%+v", err, files)
	}

	_, err = DirectoryEntries(t.Context(), sess, "dir-outside")
	if err == nil {
		t.Fatal("expected error for directory outside library source, got nil")
	}
}
