package drive

import (
	"context"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestFilesForRoutesToAccountProvider(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	used := ""
	client115 := &stubClient{
		list: func(context.Context, string, string, int, int) (pan.FilePage, error) {
			used = Provider115
			return pan.FilePage{}, nil
		},
	}
	clientGuangya := &stubClient{
		list: func(context.Context, string, string, int, int) (pan.FilePage, error) {
			used = ProviderGuangya
			return pan.FilePage{}, nil
		},
	}
	d, err := newDrive(t.Context(), store.Client, map[string]Client{
		Provider115:     client115,
		ProviderGuangya: clientGuangya,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.PanAccount.Create().
		SetAccountID("gy-account").
		SetProvider(ProviderGuangya).
		SetAccessToken("token").
		SetDirectoryID("dir").
		SetDirectoryName("Media").
		SetDirectoryPath("/Media").
		Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := d.FilesFor(t.Context(), "gy-account", "dir", 1); err != nil {
		t.Fatal(err)
	}
	if used != ProviderGuangya {
		t.Fatalf("used provider = %q, want %q", used, ProviderGuangya)
	}
}
