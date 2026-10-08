package offline

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/domain/download"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/pan"
)

type recoveryRig struct {
	t      *testing.T
	s      *Service
	client *panStub
	now    time.Time
	remote map[string]pan.OfflineTask
	events []string
	cfg    download.Config
}

func newRecoveryRig(t *testing.T) *recoveryRig {
	s, client := offlineAddFixture(t)
	r := &recoveryRig{t: t, s: s, client: client, now: time.Now(), remote: make(map[string]pan.OfflineTask), cfg: download.DefaultConfig()}
	r.cfg.ZeroProgressMinutes, r.cfg.StalledMinutes, r.cfg.CompletionGraceMinutes = 1, 1, 2
	s.now = func() time.Time { return r.now }
	r.saveConfig(magnet.DefaultPreferences())
	client.addOffline = func(_ context.Context, _, uri, directory string) (string, error) {
		hash := strings.TrimPrefix(uri, "magnet:?xt=urn:btih:")
		r.events = append(r.events, "add:"+hash)
		r.remote[hash] = pan.OfflineTask{Hash: hash, Status: 1, Progress: 21, RawProgress: 21.05, DirectoryID: directory}
		return hash, nil
	}
	client.offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
		page := pan.OfflinePage{PageCount: 1}
		for _, remote := range r.remote {
			page.Tasks = append(page.Tasks, remote)
		}
		return page, nil
	}
	client.removeOffline = func(_ context.Context, _, hash string) error {
		r.events = append(r.events, "remove:"+hash)
		delete(r.remote, hash)
		return nil
	}
	return r
}

func (r *recoveryRig) saveConfig(prefs magnet.Preferences) {
	r.t.Helper()
	if err := database.SaveSetting(r.t.Context(), r.s.database, "subscription.config", struct {
		Download    download.Config    `json:"download"`
		Preferences magnet.Preferences `json:"preferences"`
	}{r.cfg, prefs}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *recoveryRig) add(hash string) *ent.OfflineDownload {
	r.t.Helper()
	result, err := r.s.Add(r.t.Context(), "fixture-movie", hash)
	if err != nil {
		r.t.Fatal(err)
	}
	r.poll(0)
	return r.s.database.OfflineDownload.GetX(r.t.Context(), result.TaskID)
}

func (r *recoveryRig) poll(elapsed time.Duration) {
	r.t.Helper()
	r.now = r.now.Add(elapsed)
	if err := r.s.Sync(r.t.Context()); err != nil {
		r.t.Fatal(err)
	}
}

func TestRecoverySwitchesSeriallyAndPreservesTaskIdentity(t *testing.T) {
	r := newRecoveryRig(t)
	original := r.add(offlineHashA)
	r.poll(30 * time.Second)
	r.poll(30 * time.Second)
	pending := r.s.database.OfflineDownload.GetX(t.Context(), original.ID)
	if currentHash(pending) != offlineHashB || pending.Recovery.Action != actionSubmit || len(r.remote) != 0 {
		t.Fatalf("replacement was not checkpointed after cancellation: %+v, %+v", pending, r.remote)
	}
	if !slices.Equal(r.events, []string{"add:" + offlineHashA, "remove:" + offlineHashA}) {
		t.Fatal(r.events)
	}
	// Restart between cancellation and replacement submission.
	r.s = New(r.s.database, r.s.catalogue, r.s.drive, r.s.tasks, r.s.library, r.s.submitTimeout)
	r.s.now = func() time.Time { return r.now }
	r.poll(30 * time.Second)
	current := r.s.database.OfflineDownload.GetX(t.Context(), original.ID)
	if currentHash(current) != offlineHashB || current.Recovery.Action != "" || len(current.Recovery.Attempts) != 2 || len(r.remote) != 1 {
		t.Fatalf("restart failed to resume replacement: %+v", current)
	}
	if !slices.Equal(r.events, []string{"add:" + offlineHashA, "remove:" + offlineHashA, "add:" + offlineHashB}) {
		t.Fatal(r.events)
	}
	sess, err := r.s.drive.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.s.UpdateTask(t.Context(), sess, original, pan.OfflineTask{Status: 2, FileID: "old-output"}); err != nil {
		t.Fatal(err)
	}
	if got := r.s.database.OfflineDownload.GetX(t.Context(), original.ID); got.ScanTaskID != 0 || got.Status != offlinedownload.StatusRunning {
		t.Fatalf("stale completion changed replacement: %+v", got)
	}
}

func TestRecoveryDoesNotMistakeFractionalProgressForAStall(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	for range 5 {
		remote := r.remote[offlineHashA]
		remote.RawProgress += 0.01
		r.remote[offlineHashA] = remote
		r.poll(30 * time.Second)
	}
	current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
	if current.Progress != 21 || current.Recovery.Stalled || len(r.events) != 1 {
		t.Fatalf("slow download was replaced: %+v, %v", current, r.events)
	}
}

func TestRecoveryObservationsExcludeOutagesAndMissingProgress(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "outage", true: "missing progress"}[missing], func(t *testing.T) {
			r := newRecoveryRig(t)
			record := r.add(offlineHashA)
			r.poll(30 * time.Second)
			if missing {
				remote := r.remote[offlineHashA]
				remote.ProgressUnknown = true
				remote.Progress = 0
				remote.RawProgress = 0
				r.remote[offlineHashA] = remote
				r.poll(time.Minute)
			} else {
				listing := r.client.offlineTasks
				r.client.offlineTasks = func(context.Context, string, int) (pan.OfflinePage, error) {
					return pan.OfflinePage{}, context.DeadlineExceeded
				}
				r.now = r.now.Add(10 * time.Minute)
				if err := r.s.Sync(t.Context()); err == nil {
					t.Fatal("missing listing error")
				}
				r.client.offlineTasks = listing
				r.poll(0)
			}
			current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
			if current.Progress != 21 || len(r.events) != 1 || current.Recovery.Stalled {
				t.Fatalf("observation gap caused replacement: %+v", current)
			}
		})
	}
}

func TestRecoveryHonorsQueueAndCompletionGrace(t *testing.T) {
	for _, queued := range []bool{true, false} {
		t.Run(map[bool]string{true: "queued", false: "near completion"}[queued], func(t *testing.T) {
			r := newRecoveryRig(t)
			record := r.add(offlineHashA)
			remote := r.remote[offlineHashA]
			if queued {
				remote.Status, remote.Progress, remote.RawProgress = 0, 0, 0
			} else {
				remote.Progress, remote.RawProgress = 99, 99.2
			}
			r.remote[offlineHashA] = remote
			r.poll(0)
			r.poll(30 * time.Second)
			r.poll(30 * time.Second)
			if got := r.s.database.OfflineDownload.GetX(t.Context(), record.ID); got.Hash != offlineHashA || len(r.events) != 1 {
				t.Fatalf("premature switch: %+v", got)
			}
			if !queued {
				r.poll(30 * time.Second)
				r.poll(30 * time.Second)
				if len(r.events) != 2 {
					t.Fatal("completion grace never expired", r.events)
				}
			}
		})
	}
}

func TestRecoveryExhaustionPreservesCurrentDownloadAndPreferences(t *testing.T) {
	for _, limit := range []bool{false, true} {
		t.Run(map[bool]string{false: "required subtitle", true: "attempt limit"}[limit], func(t *testing.T) {
			r := newRecoveryRig(t)
			prefs := magnet.DefaultPreferences()
			if limit {
				r.cfg.MaxAttempts = 1
			} else {
				prefs.Subtitle = magnet.PreferenceRequired
			}
			r.saveConfig(prefs)
			record := r.add(offlineHashA)
			// Changing preferences later must not silently weaken this task.
			r.saveConfig(magnet.DefaultPreferences())
			r.poll(30 * time.Second)
			r.poll(30 * time.Second)
			current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
			if !current.Recovery.Exhausted || len(r.events) != 1 || current.Status != offlinedownload.StatusRunning {
				t.Fatalf("exhaustion removed the remaining download: %+v", current)
			}
			r.poll(time.Minute)
			if len(r.events) != 1 {
				t.Fatal(r.events)
			}
		})
	}
}

func TestCancelOnlyAffectsSelectedTaskAndIsIdempotent(t *testing.T) {
	r := newRecoveryRig(t)
	first := r.add(offlineHashA)
	other := r.add(offlineHashB)
	for range 2 {
		result, err := r.s.Cancel(t.Context(), first.ID)
		if err != nil || result.Status != "cancelled" || result.CanCancel {
			t.Fatalf("cancel: %+v, %v", result, err)
		}
	}
	r.poll(30 * time.Second)
	if len(r.remote) != 1 || r.remote[offlineHashB].Hash == "" || r.s.database.OfflineDownload.GetX(t.Context(), other.ID).Status != offlinedownload.StatusRunning {
		t.Fatal("cancel affected another task")
	}
	if !slices.Equal(r.events, []string{"add:" + offlineHashA, "add:" + offlineHashB, "remove:" + offlineHashA}) {
		t.Fatal(r.events)
	}
}

func TestCompletionWinsOverCancelAndSwitch(t *testing.T) {
	for _, cancel := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "switch"}[cancel], func(t *testing.T) {
			r := newRecoveryRig(t)
			record := r.add(offlineHashA)
			remote := r.remote[offlineHashA]
			remote.Status = 2
			remote.FileID = "download-folder"
			r.remote[offlineHashA] = remote
			var result domain.OfflineSubmission
			var err error
			if cancel {
				result, err = r.s.Cancel(t.Context(), record.ID)
			} else {
				result, err = r.s.TryNext(t.Context(), record.ID)
			}
			if err != nil || result.Status != "done" || result.ScanTaskID == 0 || len(r.events) != 1 {
				t.Fatalf("completion lost: %+v, %v, %v", result, err, r.events)
			}
			r.poll(30 * time.Second)
			if n := r.s.database.Task.Query().Where(task.TypeEQ("scan")).CountX(t.Context()); n != 2 {
				t.Fatalf("duplicate scans: %d", n)
			}
		})
	}
}

func TestFailedCancellationNeverSubmitsReplacement(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	failure := errors.New("fixture removal failed")
	r.client.removeOffline = func(context.Context, string, string) error { return failure }
	if _, err := r.s.TryNext(t.Context(), record.ID); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
	if current.Hash != offlineHashA || current.Recovery.Action != actionSwitch || len(r.events) != 1 {
		t.Fatalf("failed removal advanced candidate: %+v", current)
	}
	r.poll(30 * time.Second)
	if len(r.events) != 1 {
		t.Fatal(r.events)
	}
	// Cancellation may succeed remotely while its response is lost. A later
	// complete listing resolves the checkpoint without submitting in parallel.
	delete(r.remote, offlineHashA)
	r.now = current.Recovery.RetryAt
	r.poll(0)
	if len(r.events) != 1 {
		t.Fatal("replacement submitted before cancellation checkpoint", r.events)
	}
	r.poll(30 * time.Second)
	if !slices.Equal(r.events, []string{"add:" + offlineHashA, "add:" + offlineHashB}) {
		t.Fatal(r.events)
	}
}

func TestSubmissionTimeoutRecoversWithoutAnotherAdd(t *testing.T) {
	r := newRecoveryRig(t)
	add := r.client.addOffline
	r.client.addOffline = func(ctx context.Context, token, uri, dir string) (string, error) {
		_, _ = add(ctx, token, uri, dir)
		return "", context.DeadlineExceeded
	}
	if _, err := r.s.Add(t.Context(), "fixture-movie", offlineHashA); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	record := r.s.database.OfflineDownload.Query().OnlyX(t.Context())
	if record.Recovery.Action != actionSubmit || !record.Recovery.SubmissionStarted {
		t.Fatalf("submission intent lost: %+v", record)
	}
	r.s = New(r.s.database, r.s.catalogue, r.s.drive, r.s.tasks, r.s.library, r.s.submitTimeout)
	r.s.now = func() time.Time { return r.now }
	r.now = record.Recovery.RetryAt
	r.poll(0)
	current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
	if current.Recovery.Action != "" || current.InfoHash != offlineHashA || len(r.events) != 1 || len(current.Recovery.Attempts) != 1 {
		t.Fatalf("timeout duplicated submission: %+v, %v", current, r.events)
	}
}

func TestCancelBetweenSwitchAndSubmissionDoesNotStartNext(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	if _, err := r.s.TryNext(t.Context(), record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.Cancel(t.Context(), record.ID); err != nil {
		t.Fatal(err)
	}
	r.poll(time.Minute)
	if len(r.remote) != 0 || len(r.events) != 2 || r.s.database.OfflineDownload.GetX(t.Context(), record.ID).Status != offlinedownload.StatusCancelled {
		t.Fatal("cancel started next candidate", r.events)
	}
}

func TestDisablingAutoSwitchKeepsDownloadAndAllowsManualNext(t *testing.T) {
	r := newRecoveryRig(t)
	r.cfg.AutoSwitch = false
	r.saveConfig(magnet.DefaultPreferences())
	record := r.add(offlineHashA)
	r.poll(30 * time.Second)
	r.poll(30 * time.Second)
	if len(r.events) != 1 || !r.s.database.OfflineDownload.GetX(t.Context(), record.ID).Recovery.Stalled {
		t.Fatal("disabled automation changed download")
	}
	if _, err := r.s.TryNext(t.Context(), record.ID); err != nil {
		t.Fatal(err)
	}
	r.poll(30 * time.Second)
	if len(r.events) != 3 {
		t.Fatal("manual next did not run", r.events)
	}
}

func TestSwitchKeepsHistoryVisibleAndDeduplicatesCurrentCandidate(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	// A newer historical row must not shadow the older task after it switches.
	r.s.database.OfflineDownload.Create().SetHash(offlineHashB).SetInfoHash(offlineHashB).
		SetCode(record.Code).SetJavdbID(record.JavdbID).SetAccountID(record.AccountID).SetDirectoryID(record.DirectoryID).
		SetStatus(offlinedownload.StatusDone).SaveX(t.Context())
	if _, err := r.s.TryNext(t.Context(), record.ID); err != nil {
		t.Fatal(err)
	}
	r.poll(30 * time.Second)
	activity, err := r.s.Activity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(activity.Tasks, func(task domain.OfflineSubmission) bool {
		return task.TaskID == record.ID && task.Hash == offlineHashB && task.CanCancel
	}) {
		t.Fatal("switched task vanished", activity)
	}
	for _, hash := range []string{offlineHashA, offlineHashB} {
		result, err := r.s.Add(t.Context(), record.JavdbID, hash)
		if err != nil || result.TaskID != record.ID {
			t.Fatalf("candidate created duplicate: %+v, %v", result, err)
		}
	}
	if len(r.events) != 3 {
		t.Fatal(r.events)
	}
}

func TestRecoveryUsesRemoteHashToExcludeDuplicateCandidates(t *testing.T) {
	r := newRecoveryRig(t)
	add := r.client.addOffline
	r.client.addOffline = func(ctx context.Context, token, uri, dir string) (string, error) {
		_, err := add(ctx, token, uri, dir)
		remote := r.remote[offlineHashA]
		delete(r.remote, offlineHashA)
		remote.Hash = strings.ToUpper(offlineHashB)
		r.remote[offlineHashB] = remote
		return remote.Hash, err
	}
	record := r.add(offlineHashA)
	r.poll(30 * time.Second)
	r.poll(30 * time.Second)
	current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
	if !current.Recovery.Exhausted || len(r.events) != 1 {
		t.Fatalf("same info hash counted as a replacement: %+v, %v", current, r.events)
	}
}

func TestRecoveryStopsSwitchingWhenMovieWasIndexed(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	film := r.s.database.Movie.Create().SetCode(record.Code).SetJavdbID(record.JavdbID).SaveX(t.Context())
	r.s.database.File.Create().SetFileID("existing-video").SetName("existing.mp4").SetSize(1).
		SetAccountID(record.AccountID).SetRootID(record.DirectoryID).SetMovie(film).SaveX(t.Context())
	r.poll(30 * time.Second)
	r.poll(30 * time.Second)
	if got := r.s.database.OfflineDownload.GetX(t.Context(), record.ID); !got.Recovery.Exhausted || len(r.events) != 1 {
		t.Fatalf("indexed movie triggered replacement: %+v", got)
	}
}

func TestManualSwitchResumesAfterErrorWithAutomationDisabled(t *testing.T) {
	r := newRecoveryRig(t)
	r.cfg.AutoSwitch = false
	r.saveConfig(magnet.DefaultPreferences())
	record := r.add(offlineHashA)
	remove := r.client.removeOffline
	r.client.removeOffline = func(context.Context, string, string) error { return errors.New("fixture temporary error") }
	if _, err := r.s.TryNext(t.Context(), record.ID); err == nil {
		t.Fatal("expected removal error")
	}
	current := r.s.database.OfflineDownload.GetX(t.Context(), record.ID)
	r.client.removeOffline = remove
	r.now = current.Recovery.RetryAt
	r.poll(0)
	r.poll(30 * time.Second)
	if currentHash(r.s.database.OfflineDownload.GetX(t.Context(), record.ID)) != offlineHashB || len(r.events) != 3 {
		t.Fatal("manual recovery was disabled", r.events)
	}
}

func TestRecoveryReplacesExplicitFailureButDefersListingErrors(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	remote := r.remote[offlineHashA]
	remote.Status = -1
	r.remote[offlineHashA] = remote
	r.poll(30 * time.Second)
	r.poll(30 * time.Second)
	if currentHash(r.s.database.OfflineDownload.GetX(t.Context(), record.ID)) != offlineHashB || len(r.events) != 3 {
		t.Fatal("failed task was not replaced", r.events)
	}
}
