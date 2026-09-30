package pan

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLoginStatusWaitsForAnExplicitAuthorization(t *testing.T) {
	for _, test := range []struct {
		body    string
		want    LoginState
		wantErr bool
	}{
		{`{"state":1,"code":0,"data":{}}`, LoginWaiting, false},
		{`{"state":1,"code":0}`, LoginWaiting, false},
		{`{"state":1,"code":0,"data":{"status":null}}`, LoginWaiting, false},
		{`{"state":1,"code":0,"data":{"status":0}}`, LoginWaiting, false},
		{`{"state":1,"code":0,"data":{"status":1}}`, LoginScanned, false},
		{`{"state":1,"code":0,"data":{"status":2}}`, LoginAuthorized, false},
		{`{"state":1,"code":0,"data":{"status":-1}}`, LoginExpired, false},
		{`{"state":1,"code":0,"data":{"status":-2}}`, LoginCanceled, false},
		{`{"state":0,"code":99}`, "", true},
		{`{"state":1,"code":0,"data":{"status":99}}`, LoginWaiting, false},
	} {
		client := New()
		client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": {"application/json"}},
				Body:   io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
		}))
		got, err := client.LoginStatus(t.Context(), &Login{uid: "fixture", time: 1, sign: "fixture"})
		client.Close()
		if got != test.want || (err != nil) != test.wantErr {
			t.Errorf("body %s: state=%s error=%v", test.body, got, err)
		}
	}
}
