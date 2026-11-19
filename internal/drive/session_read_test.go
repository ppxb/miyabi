package drive

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestOpenSourceReads(t *testing.T) {
	d, client := mountedTestDrive(t)
	client.info = func(_ context.Context, _, id string) (pan.FileInfo, error) {
		return pan.FileInfo{File: pan.File{ID: id, Name: "video.mp4"}}, nil
	}
	sess, err := d.OpenSource(t.Context(), testSource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.List(t.Context(), testSource.Directory.ID, 0); err != nil {
		t.Fatal(err)
	}
	info, err := sess.Info(t.Context(), "f1")
	if err != nil || info.ID != "f1" {
		t.Fatalf("info = %+v err=%v", info, err)
	}
}

func TestOpenSourceRejectsChangedMount(t *testing.T) {
	d, _ := mountedTestDrive(t)
	sess, err := d.OpenSource(t.Context(), testSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ClearDirectoryForAccount(t.Context(), testSource.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.List(t.Context(), testSource.Directory.ID, 0); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("read after unmount = %v, want ErrSourceChanged", err)
	}
}
