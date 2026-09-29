package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type panResponseFixture struct {
	DriveManager
	page pan.FilePage
}

func (fixture panResponseFixture) Files(context.Context, string, int) (pan.FilePage, error) {
	return fixture.page, nil
}

func (panResponseFixture) Disconnect(context.Context) (drive.AccountStatus, error) {
	return drive.AccountStatus{}, nil
}

func TestPanFilesResponsePreservesJSONContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		page pan.FilePage
		want string
	}{
		{name: "nil lists", want: `{"files":null,"path":null,"total":0,"has_more":false}`},
		{name: "empty lists", page: pan.FilePage{Files: []pan.File{}, Path: []pan.Directory{}},
			want: `{"files":[],"path":[],"total":0,"has_more":false}`},
		{name: "populated", page: pan.FilePage{
			Files: []pan.File{{ID: "10", ParentID: "1", Name: "movie.mp4", Size: 1024, PickCode: "pick", SHA1: "hash"},
				{ID: "11", ParentID: "1", Name: "Movies", IsDirectory: true}},
			Path: []pan.Directory{{ID: "0", Name: "root"}, {ID: "1", Name: "Media"}}, Total: 3, HasMore: true,
		}, want: `{"files":[{"id":"10","parent_id":"1","name":"movie.mp4","is_directory":false,"size":1024,"pick_code":"pick","sha1":"hash"},{"id":"11","parent_id":"1","name":"Movies","is_directory":true,"size":0,"pick_code":"","sha1":""}],"path":[{"id":"0","name":"root"},{"id":"1","name":"Media"}],"total":3,"has_more":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := errorRouter(panFilesHandler(panResponseFixture{page: tc.page}))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", response.Code, response.Body)
			}
			var got, want any
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response = %s, want %s", response.Body, tc.want)
			}
		})
	}
}

func TestPanDisconnectOmitsAbsentAccountAndDirectory(t *testing.T) {
	router := errorRouter(panDisconnectHandler(panResponseFixture{}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"connected":false}` {
		t.Fatalf("unexpected disconnect response: %d %s", response.Code, response.Body)
	}
}
