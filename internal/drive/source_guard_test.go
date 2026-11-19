package drive

import (
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestOpenSourceRejectsMismatchedDirectory(t *testing.T) {
	d, _ := mountedTestDrive(t)
	mismatched := domain.LibrarySource{
		AccountID: testSource.AccountID,
		Directory: domain.LibraryDirectory{ID: "99", Name: "Other", Path: "/Other"},
	}
	if _, err := d.OpenSource(t.Context(), mismatched); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("open mismatched source = %v, want ErrSourceChanged", err)
	}
}
