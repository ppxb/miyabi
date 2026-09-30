package emby

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestClientUsesHeaderAuthAndPreservesRequestFormats(t *testing.T) {
	paths := make(chan string, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "secret +&" || r.URL.RawQuery != "" {
			t.Errorf("incorrect auth: header=%q query=%q", r.Header.Get("X-Emby-Token"), r.URL.RawQuery)
		}
		paths <- r.URL.Path
		switch r.URL.Path {
		case "/emby/System/Info":
			if r.Method != http.MethodGet {
				t.Errorf("method=%s", r.Method)
			}
			io.WriteString(w, `{"ServerName":"Emby","Version":"4.8","Id":"server"}`)
		case "/emby/Persons":
			io.WriteString(w, `{"Items":[{"Id":"one","Name":"Actor"},{"Id":"two","Name":"Existing","ImageTags":{"Primary":"tag"}},{"Name":"No ID"}]}`)
		case "/emby/Library/Media/Updated":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("invalid notification request")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"Updates":[{"Path":"/media/one","UpdateType":"Created"}]}` {
				t.Errorf("body=%s", body)
			}
			w.WriteHeader(http.StatusNoContent)
		case "/emby/Items/one/Images/Primary":
			body, _ := io.ReadAll(r.Body)
			if string(body) != base64.StdEncoding.EncodeToString([]byte("image")) || r.Header.Get("Content-Type") != "image/jpeg" {
				t.Errorf("avatar format changed")
			}
			w.WriteHeader(http.StatusNoContent)
		case "/emby/Library/Refresh":
			if r.Method != http.MethodPost {
				t.Errorf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newEmbyClient()
	cfg := Config{ServerURL: server.URL + "/emby/", APIKey: "secret +&"}
	info, err := client.ping(t.Context(), cfg)
	if err != nil || info.ID != "server" || info.Version != "4.8" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	people, err := client.persons(t.Context(), cfg)
	if err != nil || len(people) != 2 || people[0].ID != "one" || people[1].ID != "two" {
		t.Fatalf("people=%+v err=%v", people, err)
	}
	if err := client.notify(t.Context(), cfg, []mediaUpdateItem{{Path: "/media/one", UpdateType: "Created"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.uploadAvatar(t.Context(), cfg, "one", domain.Media{Body: []byte("image"), ContentType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	if err := client.refresh(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 5 {
		t.Fatalf("requests=%d", len(paths))
	}
}

func TestClientClassifiesFailuresAndPreservesCancellation(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			_, err := newEmbyClient().ping(t.Context(), Config{ServerURL: server.URL})
			want := domain.KindUpstream
			if status == 401 || status == 403 {
				want = domain.KindUnauthorized
			}
			if !domain.IsKind(err, want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := newEmbyClient().ping(ctx, Config{ServerURL: "http://127.0.0.1:1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestAvatarHealthUsesDecodedDimensionsInsteadOfImageTagsOrSize(t *testing.T) {
	for _, scenario := range []struct {
		name, details string
		status        int
		valid         bool
	}{
		{"valid zero-size metadata", `[{"ImageType":"Primary","Width":120,"Height":180,"Size":0}]`, 200, true},
		{"encoded image", `[{"ImageType":"Primary","Size":0}]`, 200, false},
		{"missing file", `[]`, 200, false},
		{"other image type", `[{"ImageType":"Backdrop","Width":120,"Height":180}]`, 200, false},
		{"inspection failed", `server unavailable`, 503, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/Items/person/Images" || r.Method != http.MethodGet {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(scenario.status)
				io.WriteString(w, scenario.details)
			}))
			defer server.Close()
			for _, person := range []personItem{{ID: "person", PrimaryImageTag: "tag"}, {ID: "person", ImageTags: map[string]string{"Primary": "tag"}}} {
				valid, err := newEmbyClient().hasValidAvatar(t.Context(), Config{ServerURL: server.URL}, person)
				if valid != scenario.valid || (err != nil) != (scenario.status != 200) {
					t.Fatalf("valid=%t err=%v", valid, err)
				}
			}
		})
	}
}
