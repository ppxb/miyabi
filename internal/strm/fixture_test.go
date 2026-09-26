package strm

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

// panStub implements the 115 calls used by login, mounting, and the relay.
// Any other call panics through the nil embedded client.
type panStub struct {
	drive.Client
	info      func(context.Context, string, string) (pan.FileInfo, error)
	playURL   func(context.Context, string, string) ([]pan.PlaySource, error)
	openMedia func(context.Context, string, string, http.Header) (*http.Response, error)
}

func (*panStub) Close() {}

func (*panStub) Account(context.Context, string) (pan.Account, error) {
	return pan.Account{ID: "100"}, nil
}

func (*panStub) BeginLogin(context.Context) (*pan.Login, error) {
	return &pan.Login{QRCode: []byte("fixture")}, nil
}

func (*panStub) LoginStatus(context.Context, *pan.Login) (pan.LoginState, error) {
	return pan.LoginAuthorized, nil
}

func (*panStub) ExchangeToken(context.Context, *pan.Login) (pan.Tokens, error) {
	return pan.Tokens{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (*panStub) List(context.Context, string, string, int, int) (pan.FilePage, error) {
	return pan.FilePage{Path: []pan.Directory{{ID: "0", Name: "Root"}, {ID: "10", Name: "Movies"}}}, nil
}

func (client *panStub) Info(ctx context.Context, token, id string) (pan.FileInfo, error) {
	return client.info(ctx, token, id)
}

func (client *panStub) PlayURL(ctx context.Context, token, pickCode string) ([]pan.PlaySource, error) {
	return client.playURL(ctx, token, pickCode)
}

func (client *panStub) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	return client.openMedia(ctx, method, address, headers)
}

// relayFixture mounts directory 10 of account 100 and indexes video 101 with the given pick code.
func relayFixture(t *testing.T, pickCode string) (*Relay, *panStub) {
	t.Helper()
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	client := &panStub{}
	d, err := drive.NewWithClient(ctx, store.Client, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	login, err := d.BeginLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := d.LoginStatus(ctx, login.ID); err != nil || status.State != pan.LoginAuthorized {
		t.Fatalf("fixture login = %+v, %v", status, err)
	}
	if _, err := d.SelectDirectory(ctx, "10"); err != nil {
		t.Fatal(err)
	}
	film := store.Client.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	store.Client.File.Create().SetFileID("101").SetName("ABP-001.mp4").SetSize(1 << 30).SetPickCode(pickCode).
		SetAccountID("100").SetRootID("10").SetMovie(film).ExecX(ctx)
	return New(store.Client, d), client
}
