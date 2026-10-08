package drive

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestSessionSourceReads(t *testing.T) {
	failure := errors.New("read failed after source changed")
	for _, operation := range []struct {
		name string
		stub func(*stubClient, func(context.Context, string) error)
		call func(context.Context, Session) (any, error)
		want any
	}{
		{
			name: "list",
			stub: func(client *stubClient, request func(context.Context, string) error) {
				client.list = func(ctx context.Context, token, _ string, _, _ int) (pan.FilePage, error) {
					return pan.FilePage{Files: []pan.File{{ID: "f1"}}, Total: 1}, request(ctx, token)
				}
			},
			call: func(ctx context.Context, sess Session) (any, error) { return sess.List(ctx, "10", 0) },
			want: pan.FilePage{Files: []pan.File{{ID: "f1"}}, Total: 1},
		},
		{
			name: "info",
			stub: func(client *stubClient, request func(context.Context, string) error) {
				client.info = func(ctx context.Context, token, _ string) (pan.FileInfo, error) {
					return pan.FileInfo{File: pan.File{ID: "f1"}}, request(ctx, token)
				}
			},
			call: func(ctx context.Context, sess Session) (any, error) { return sess.Info(ctx, "f1") },
			want: pan.FileInfo{File: pan.File{ID: "f1"}},
		},
		{
			name: "read",
			stub: func(client *stubClient, request func(context.Context, string) error) {
				client.readMetadata = func(ctx context.Context, token, _ string, _ int64) ([]byte, error) {
					return []byte("<movie/>"), request(ctx, token)
				}
			},
			call: func(ctx context.Context, sess Session) (any, error) { return sess.Read(ctx, "pick", 1024) },
			want: []byte("<movie/>"),
		},
		{
			name: "play",
			stub: func(client *stubClient, request func(context.Context, string) error) {
				client.playURL = func(ctx context.Context, token, _, _ string) ([]pan.PlaySource, error) {
					return []pan.PlaySource{{URL: "https://cdn.example/play"}}, request(ctx, token)
				}
			},
			call: func(ctx context.Context, sess Session) (any, error) { return sess.PlayURL(ctx, "pick", "player") },
			want: []pan.PlaySource{{URL: "https://cdn.example/play"}},
		},
		{
			name: "download",
			stub: func(client *stubClient, request func(context.Context, string) error) {
				client.downloadURL = func(ctx context.Context, token, _ string) (string, error) {
					return "https://cdn.example/download", request(ctx, token)
				}
			},
			call: func(ctx context.Context, sess Session) (any, error) { return sess.DownloadURL(ctx, "pick", "player") },
			want: "https://cdn.example/download",
		},
	} {
		for _, tc := range []struct {
			name                string
			err                 error
			requests, refreshes int
		}{
			{name: "success", requests: 1},
			{name: "stale_before_refresh", err: ErrSourceChanged},
			{name: "source_changed_during_read", err: ErrSourceChanged, requests: 1},
			{name: "disconnect_during_read", err: ErrSourceChanged, requests: 1},
			{name: "read_error_after_source_change", err: failure, requests: 1},
			{name: "canceled_before_read", err: context.Canceled},
			{name: "canceled_during_read", err: context.Canceled, requests: 1},
			{name: "token_refresh", requests: 1, refreshes: 1},
			{name: "source_changed_during_refresh", err: ErrSourceChanged, refreshes: 1},
		} {
			t.Run(operation.name+"/"+tc.name, func(t *testing.T) {
				d, client := mountedTestDrive(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				sess, err := d.Open(ctx)
				if err != nil {
					t.Fatal(err)
				}
				clearDirectory := func() {
					t.Helper()
					if err := d.ClearDirectory(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				wantToken := d.snapshot().tokens.AccessToken
				refreshed := testTokens("rotated")
				requests, refreshes := 0, 0
				client.refreshToken = func(context.Context, string) (pan.Tokens, error) {
					refreshes++
					if tc.name == "source_changed_during_refresh" {
						clearDirectory()
					}
					return refreshed, nil
				}
				operation.stub(client, func(ctx context.Context, token string) error {
					requests++
					if token != wantToken {
						t.Error("request used unexpected credentials")
					}
					switch tc.name {
					case "source_changed_during_read":
						clearDirectory()
					case "disconnect_during_read":
						if _, err := d.Disconnect(ctx); err != nil {
							t.Fatal(err)
						}
					case "read_error_after_source_change":
						clearDirectory()
						return failure
					case "canceled_during_read":
						cancel()
						return ctx.Err()
					}
					return nil
				})
				switch tc.name {
				case "stale_before_refresh":
					clearDirectory()
					expireTokens(d)
				case "canceled_before_read":
					cancel()
				case "token_refresh", "source_changed_during_refresh":
					expireTokens(d)
					wantToken = refreshed.AccessToken
				}
				value, err := operation.call(ctx, sess)
				if !errors.Is(err, tc.err) {
					t.Fatalf("error = %v, want %v", err, tc.err)
				}
				if tc.err != nil {
					if !reflect.ValueOf(value).IsZero() {
						t.Errorf("failed read exposed a result: %+v", value)
					}
				} else if !reflect.DeepEqual(value, operation.want) {
					t.Errorf("result = %+v, want %+v", value, operation.want)
				}
				if requests != tc.requests || refreshes != tc.refreshes {
					t.Errorf("requests = %d, refreshes = %d; want %d, %d", requests, refreshes, tc.requests, tc.refreshes)
				}
			})
		}
	}
}
