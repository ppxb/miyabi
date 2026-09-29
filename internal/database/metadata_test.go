package database

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestMetadataSnapshotMigrationPreservesLatestScopedExport(t *testing.T) {
	directory := t.TempDir()
	ctx := t.Context()
	store, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	film := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("preserved").SaveX(ctx)
	store.Client.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetAccountID("100").SetRootID("10").SetMovie(film).SaveX(ctx)
	var jobs []*ent.Task
	add := func(status task.Status, account, root, snapshot string) *ent.Task {
		t.Helper()
		payload := json.RawMessage(fmt.Sprintf(`{"movie_id":%d,"source":{"account_id":%q,"directory":{"id":%q}},"snapshot":%s,"scan_task_id":9007199254740993,"future":{"keep":true}}`, film.ID, account, root, snapshot))
		job := store.Client.Task.Create().SetType("cover").SetStatus(status).SetProgress(67).
			SetCreatedAt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetPayload(payload).SaveX(ctx)
		jobs = append(jobs, job)
		return job
	}
	add(task.StatusDone, "100", "10", `{"videos":"old"}`)
	latest := add(task.StatusDone, "100", "10", `{"videos":"latest","directories":[{"id":"11","nfo":{"name":"ABP-001.nfo","sha1":"nfo"},"poster":{"name":"poster.jpg","sha1":"poster"},"fanart":{"name":"fanart.jpg","sha1":"fanart"}}]}`)
	add(task.StatusDone, "other-account", "10", `{"videos":"other-account"}`)
	add(task.StatusDone, "100", "other-root", `{"videos":"other-root"}`)
	add(task.StatusQueued, "100", "10", `null`)
	add(task.StatusFailed, "100", "10", `null`)
	// Recreate the previous schema and remove the one-time migration marker.
	store.Client.Setting.Delete().Where(setting.Key(metadataSnapshotMigration)).ExecX(ctx)
	setMigrationVersion(t, store, 3)
	if _, err := store.db.ExecContext(ctx, `ALTER TABLE movies DROP COLUMN metadata_snapshot`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Client.Movie.GetX(ctx, film.ID)
	want := &domain.MetadataSnapshot{AccountID: "100", DirectoryID: "10", Videos: "latest", Directories: []domain.DirectorySnapshot{{ID: "11",
		NFO: domain.Sidecar{Name: "ABP-001.nfo", SHA1: "nfo"}, Poster: domain.Sidecar{Name: "poster.jpg", SHA1: "poster"}, Fanart: domain.Sidecar{Name: "fanart.jpg", SHA1: "fanart"}}}}
	if !reflect.DeepEqual(got.MetadataSnapshot, want) || got.Title != film.Title || !got.UpdatedAt.Equal(film.UpdatedAt) {
		t.Fatalf("migrated movie = %+v", got)
	}
	for _, job := range jobs[:4] {
		saved := store.Client.Task.GetX(ctx, job.ID)
		var input struct {
			Completed  bool            `json:"completed"`
			Snapshot   json.RawMessage `json:"snapshot"`
			ScanTaskID int64           `json:"scan_task_id"`
			Future     json.RawMessage `json:"future"`
		}
		if err := json.Unmarshal(saved.Payload, &input); err != nil {
			t.Fatal(err)
		}
		if !input.Completed || input.Snapshot != nil || input.ScanTaskID != 9007199254740993 || string(input.Future) != `{"keep":true}` ||
			saved.Status != job.Status || saved.Progress != job.Progress || !saved.UpdatedAt.Equal(job.UpdatedAt) || !saved.CreatedAt.Equal(job.CreatedAt) {
			t.Fatalf("cover checkpoint changed unrelated state: %+v, %s", saved, saved.Payload)
		}
	}
	// Completed tasks can be removed without affecting the saved export after restart.
	store.Client.Task.DeleteOneID(latest.ID).ExecX(ctx)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Client.Movie.GetX(ctx, film.ID); !reflect.DeepEqual(got.MetadataSnapshot, want) {
		t.Fatalf("reopen replaced snapshot: %+v", got.MetadataSnapshot)
	}
}

func TestMetadataSnapshotMigrationHandlesLegacyCompletion(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		status   task.Status
		snapshot string
		want     bool
	}{
		{"pending queue completion", task.StatusRunning, `{"videos":"committed","local_export":true}`, true},
		{"recovered queue completion", task.StatusQueued, `{"videos":"committed","local_export":true}`, true},
		{"newer completion without snapshot", task.StatusDone, `null`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := t.Context()
			store, err := Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			film := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
			store.Client.File.Create().SetFileID("video").SetName("video.mp4").SetSize(1 << 30).
				SetAccountID("100").SetRootID("10").SetMovie(film).SaveX(ctx)
			store.Client.Task.Create().SetType("cover").SetStatus(task.StatusDone).SetPayload(json.RawMessage(fmt.Sprintf(`{"movie_id":%d,"source":{"account_id":"100","directory":{"id":"10"}},"snapshot":{"videos":"old"}}`, film.ID))).SaveX(ctx)
			job := store.Client.Task.Create().SetType("cover").SetStatus(scenario.status).SetPayload(json.RawMessage(fmt.Sprintf(`{"movie_id":%d,"source":{"account_id":"100","directory":{"id":"10"}},"snapshot":%s}`, film.ID, scenario.snapshot))).SaveX(ctx)
			store.Client.Setting.Delete().Where(setting.Key(metadataSnapshotMigration)).ExecX(ctx)
			setMigrationVersion(t, store, 3)
			if err := runMigrations(ctx, store.db, len(migrations)); err != nil {
				t.Fatal(err)
			}
			snapshot := store.Client.Movie.GetX(ctx, film.ID).MetadataSnapshot
			if scenario.want {
				if snapshot == nil || snapshot.Videos != "committed" || !snapshot.LocalExport {
					t.Fatalf("lost committed export: %+v", snapshot)
				}
				var checkpoint struct {
					Completed bool `json:"completed"`
				}
				if err := json.Unmarshal(store.Client.Task.GetX(ctx, job.ID).Payload, &checkpoint); err != nil || !checkpoint.Completed {
					t.Fatalf("lost recovery checkpoint: %+v, %v", checkpoint, err)
				}
			} else if snapshot != nil {
				t.Fatalf("reused an obsolete snapshot: %+v", snapshot)
			}
		})
	}
}

func TestMetadataSnapshotMigrationRollsBackInvalidHistory(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	film := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	store.Client.File.Create().SetFileID("video").SetName("video.mp4").SetSize(1 << 30).
		SetAccountID("100").SetRootID("10").SetMovie(film).SaveX(ctx)
	good := store.Client.Task.Create().SetType("cover").SetStatus(task.StatusDone).
		SetPayload(json.RawMessage(fmt.Sprintf(`{"movie_id":%d,"source":{"account_id":"100","directory":{"id":"10"}},"snapshot":{"videos":"valid"}}`, film.ID))).SaveX(ctx)
	bad := store.Client.Task.Create().SetType("cover").SetStatus(task.StatusDone).SetPayload(json.RawMessage(`{"snapshot":{"videos":123}}`)).SaveX(ctx)
	store.Client.Setting.Delete().Where(setting.Key(metadataSnapshotMigration)).ExecX(ctx)
	setMigrationVersion(t, store, 3)
	if err := runMigrations(ctx, store.db, len(migrations)); err == nil || !strings.Contains(err.Error(), fmt.Sprint(bad.ID)) {
		t.Fatalf("invalid history accepted: %v", err)
	}
	if got := migrationVersion(t, store.db); got != 3 {
		t.Fatalf("failed metadata migration advanced version to %d", got)
	}
	if got := store.Client.Movie.GetX(ctx, film.ID); got.MetadataSnapshot != nil {
		t.Fatal("snapshot escaped rollback")
	}
	if got := store.Client.Task.GetX(ctx, good.ID); string(got.Payload) != string(good.Payload) {
		t.Fatal("checkpoint escaped rollback")
	}
	if store.Client.Setting.Query().Where(setting.Key(metadataSnapshotMigration)).ExistX(ctx) {
		t.Fatal("failed migration marked complete")
	}
	store.Client.Task.DeleteOne(bad).ExecX(ctx)
	if err := runMigrations(ctx, store.db, len(migrations)); err != nil {
		t.Fatal(err)
	}
	if got := store.Client.Movie.GetX(ctx, film.ID); got.MetadataSnapshot == nil || got.MetadataSnapshot.Videos != "valid" {
		t.Fatal("migration could not resume")
	}
}

func TestVersionedMigrationHonorsLegacyMetadataMarker(t *testing.T) {
	directory := t.TempDir()
	ctx := t.Context()
	store, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	snapshot := &domain.MetadataSnapshot{AccountID: "100", DirectoryID: "10", Videos: "already-migrated"}
	film := store.Client.Movie.Create().SetCode("ABP-001").SetMetadataSnapshot(snapshot).SaveX(ctx)
	if err := SaveSetting(ctx, store.Client, metadataSnapshotMigration, true); err != nil {
		t.Fatal(err)
	}
	// This leftover history would fail decoding if an already-completed legacy
	// snapshot migration ran again during adoption of user_version.
	store.Client.Task.Create().SetType("cover").SetStatus(task.StatusDone).
		SetPayload(json.RawMessage(`{"snapshot":{"videos":123}}`)).SaveX(ctx)
	setMigrationVersion(t, store, 0)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Client.Movie.GetX(ctx, film.ID); !reflect.DeepEqual(got.MetadataSnapshot, snapshot) {
		t.Fatalf("overwrote existing snapshot: %+v", got.MetadataSnapshot)
	}
	if got := migrationVersion(t, store.db); got != len(migrations) {
		t.Fatalf("legacy migration version=%d", got)
	}
}
