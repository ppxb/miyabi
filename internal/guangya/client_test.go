package guangya

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewWithBases(server.URL, server.URL)
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("content-type", "application/json")
	_, _ = io.WriteString(w, body)
}

func TestAccount(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/user/me" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("authorization") != "Bearer token" {
			t.Errorf("authorization = %q", r.Header.Get("authorization"))
		}
		writeJSON(w, `{"sub":"user-1","name":"Tester"}`)
	})
	account, err := client.Account(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if account.ID != "user-1" || account.Name != "Tester" {
		t.Fatalf("account = %+v", account)
	}
}

func TestListUsesFileIDAsPickCodeAndBuildsPath(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/userres/v1/file/get_file_list":
			writeJSON(w, `{"msg":"success","data":{"total":1,"list":[{
				"fileId":"100","parentId":"10","fileName":"Movie.mp4","fileSize":"1024",
				"resType":1,"fileType":2,"ctime":1700000000,"gcid":"GCID100",
				"fullParentIds":"10/"}]}}`)
		case "/nd.bizuserres.s/v1/file/get_file_detail":
			writeJSON(w, `{"msg":"success","data":{"fileInfo":{"fileId":"10","fileName":"Movies","resType":2}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	page, err := client.List(context.Background(), "token", "10", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Files) != 1 {
		t.Fatalf("files = %d", len(page.Files))
	}
	file := page.Files[0]
	if file.ID != "100" || file.PickCode != "100" || file.GCID != "GCID100" || file.Size != 1024 {
		t.Fatalf("file = %+v", file)
	}
	if len(page.Path) != 1 || page.Path[0].ID != "10" || page.Path[0].Name != "Movies" {
		t.Fatalf("path = %+v", page.Path)
	}
	if page.Total != 1 || page.HasMore {
		t.Fatalf("page = %+v", page)
	}
}

func TestInfoIncludesVideoGCID(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nd.bizuserres.s/v1/file/get_file_detail" {
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		if strings.Contains(r.URL.Path, "get_file_detail") {
			writeJSON(w, `{"msg":"success","data":{
				"fileInfo":{"fileId":"100","fileName":"Movie.mp4","fileSize":"1024","resType":1,"fullParentIds":"10/"},
				"videoResource":[{"gcid":"VIDEO-GCID","info":{"duration":100}}]}}`)
		}
	})
	info, err := client.Info(context.Background(), "token", "100")
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "100" || info.PickCode != "100" || info.GCID != "VIDEO-GCID" {
		t.Fatalf("info = %+v", info)
	}
	if len(info.Path) != 1 || info.Path[0].ID != "10" {
		t.Fatalf("path = %+v", info.Path)
	}
}

func TestDownloadURLMapsSizeLimitForSidecar(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nd.bizuserres.s/v1/file/get_file_detail":
			writeJSON(w, `{"msg":"success","data":{"fileInfo":{"fileId":"100","fileName":"movie.nfo","resType":1}}}`)
		case "/nd.bizuserres.s/v1/get_res_download_url":
			writeJSON(w, `{"code":254,"msg":"too large"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	_, err := client.DownloadURL(context.Background(), "token", "100", "")
	if !errors.Is(err, pan.ErrDownloadUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadURLUsesVODForVideo(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nd.bizuserres.s/v1/file/get_file_detail":
			writeJSON(w, `{"msg":"success","data":{
				"fileInfo":{"fileId":"100","fileName":"Movie.mp4","resType":1,"fullParentIds":"10/"},
				"videoResource":[{"gcid":"VIDEO-GCID"}]}}`)
		case "/nd.bizuserres.s/v1/file/get_vod_download_url":
			writeJSON(w, `{"msg":"success","data":{"signedURL":"https://cdn.example/vod.mp4","urlDuration":21600}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	address, err := client.DownloadURL(context.Background(), "token", "100", "")
	if err != nil {
		t.Fatal(err)
	}
	if address != "https://cdn.example/vod.mp4" {
		t.Fatalf("address = %q", address)
	}
}

func TestAddOfflineReturnsMagnetHash(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nd.bizcloudcollection.s/v1/resolve_res":
			writeJSON(w, `{"msg":"success","data":{"url":"magnet:?xt=urn:btih:62cb85b6c39d7038b330ef922fe7bb830394e311"}}`)
		case "/nd.bizcloudcollection.s/v1/create_task":
			writeJSON(w, `{"msg":"success","data":{"taskId":"task-1"}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	hash, err := client.AddOffline(context.Background(), "token",
		"magnet:?xt=urn:btih:62CB85B6C39D7038B330EF922FE7BB830394E311&dn=x", "10")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "62cb85b6c39d7038b330ef922fe7bb830394e311" {
		t.Fatalf("hash = %q", hash)
	}
}

func TestOfflineTasksMapsStatusAndPagination(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if _, ok := body["taskIds"]; ok {
			t.Error("empty taskIds must be omitted; upstream rejects it with code 112")
		}
		if size, _ := body["pageSize"].(float64); size <= 0 || size > 50 {
			t.Errorf("pageSize = %v, want 1..50", body["pageSize"])
		}
		writeJSON(w, `{"msg":"success","data":{"cursor":"","list":[
			{"taskId":"t1","status":1,"progress":45.5,"fileName":"a.mp4","totalSize":"100",
			 "fileId":"f1","isDir":false,"res":"magnet:?xt=urn:btih:62CB85B6C39D7038B330EF922FE7BB830394E311"},
			{"taskId":"t2","status":2,"fileName":"b.mp4","totalSize":"200","fileId":"f2","isDir":true,
			 "res":"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
		]}}`)
	})
	page, err := client.OfflineTasks(context.Background(), "token", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.PageCount != 1 || len(page.Tasks) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if page.Tasks[0].Hash != "62cb85b6c39d7038b330ef922fe7bb830394e311" ||
		page.Tasks[0].Status != pan.OfflineStatusRunning || page.Tasks[0].Progress != 45 {
		t.Fatalf("task0 = %+v", page.Tasks[0])
	}
	if page.Tasks[1].Status != pan.OfflineStatusDone || page.Tasks[1].FileID != "f2" {
		t.Fatalf("task1 = %+v", page.Tasks[1])
	}
	empty, err := client.OfflineTasks(context.Background(), "token", 2)
	if err != nil || len(empty.Tasks) != 0 {
		t.Fatalf("page2 = %+v, err = %v", empty, err)
	}
}

func TestRemoveOfflineFindsTaskByHash(t *testing.T) {
	var deleted []string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nd.bizcloudcollection.s/v1/list_task":
			writeJSON(w, `{"msg":"success","data":{"cursor":"","list":[
				{"taskId":"t1","status":2,"fileId":"f1","res":"magnet:?xt=urn:btih:62CB85B6C39D7038B330EF922FE7BB830394E311"}]}}`)
		case "/nd.bizcloudcollection.s/v2/delete_task":
			body, _ := io.ReadAll(r.Body)
			deleted = append(deleted, string(body))
			writeJSON(w, `{"msg":"success","data":{}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	err := client.RemoveOffline(context.Background(), "token",
		"62cb85b6c39d7038b330ef922fe7bb830394e311")
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || !strings.Contains(deleted[0], "t1") {
		t.Fatalf("deleted = %+v", deleted)
	}
}

func TestMagnetHash(t *testing.T) {
	hash, ok := magnetHash("magnet:?xt=urn:btih:62CB85B6C39D7038B330EF922FE7BB830394E311&dn=x")
	if !ok || hash != "62cb85b6c39d7038b330ef922fe7bb830394e311" {
		t.Fatalf("hash = %q, ok = %v", hash, ok)
	}
	if _, ok := magnetHash("magnet:?xt=urn:btih:not-hex"); ok {
		t.Fatal("accepted invalid hash")
	}
}
