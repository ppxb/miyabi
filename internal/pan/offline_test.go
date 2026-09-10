package pan

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type offlineRoundTrip func(*http.Request) (*http.Response, error)

func (roundTrip offlineRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestAddOfflineChecksTheIndividualSubmissionResult(t *testing.T) {
	hash := strings.Repeat("a", 40)
	for _, test := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "accepted",
			body: fmt.Sprintf(`{"state":true,"code":0,"data":[{"state":true,"code":0,"info_hash":%q}]}`, hash),
		},
		{
			name:    "individual rejection",
			body:    `{"state":true,"code":0,"data":[{"state":false,"code":500001,"message":"fixture rejection"}]}`,
			wantErr: true,
		},
		{name: "missing result", body: `{"state":true,"code":0,"data":[]}`, wantErr: true},
		{name: "missing task hash", body: `{"state":true,"code":0,"data":[{"state":true,"code":0}]}`, wantErr: true},
		{name: "global rejection", body: `{"state":false,"code":500001,"message":"fixture rejection"}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := New(Options{})
			defer client.Close()
			calls := 0
			client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Method != http.MethodPost || request.URL.Path != "/open/offline/add_task_urls" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
				}
				if request.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("missing bearer authorization")
				}
				if err := request.ParseMultipartForm(1 << 20); err != nil {
					t.Errorf("multipart form: %v", err)
				}
				if request.FormValue("urls") != "magnet:?xt=urn:btih:"+hash || request.FormValue("wp_path_id") != "42" {
					t.Error("magnet or target directory was not sent")
				}
				return &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(test.body)), Request: request,
				}, nil
			}))
			got, err := client.AddOffline(t.Context(), "fixture-token", "magnet:?xt=urn:btih:"+hash, "42")
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, want error = %t", err, test.wantErr)
			}
			if !test.wantErr && got != hash {
				t.Fatalf("returned task hash = %q", got)
			}
			if calls != 1 {
				t.Fatalf("submission was sent %d times", calls)
			}
		})
	}
}

func TestOfflineTasksDecodesMixedProgress(t *testing.T) {
	for _, progress := range []string{"42", "42.75", `"42.75"`} {
		t.Run(progress, func(t *testing.T) {
			client := New(Options{})
			defer client.Close()
			client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Path != "/open/offline/get_task_list" || request.URL.Query().Get("page") != "2" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL)
				}
				body := fmt.Sprintf(`{"state":true,"code":0,"data":{"page_count":2,"tasks":[
					{"info_hash":"downloading","status":1,"percentDone":%s,"wp_path_id":"42"},
					{"info_hash":"completed","status":2,"percentDone":100.0,"file_id":"video","wp_path_id":"42"},
					{"info_hash":"queued","status":0,"percentDone":null,"wp_path_id":"42"}
				]}}`, progress)
				return &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(body)), Request: request,
				}, nil
			}))
			page, err := client.OfflineTasks(t.Context(), "fixture-token", 2)
			if err != nil {
				t.Fatalf("one in-progress download blocked the completed task: %v", err)
			}
			if page.PageCount != 2 || len(page.Tasks) != 3 || page.Tasks[0].Hash != "downloading" ||
				page.Tasks[0].Progress != 42 || page.Tasks[0].DirectoryID != "42" ||
				page.Tasks[1].Status != 2 || page.Tasks[1].Progress != 100 || page.Tasks[1].FileID != "video" ||
				page.Tasks[2].Progress != 0 {
				t.Fatalf("unexpected offline page: %+v", page)
			}
		})
	}
}
