package javdb

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

type responseClient struct {
	response *http.Response
}

func (client *responseClient) Do(*http.Request) (*http.Response, error) {
	return client.response, nil
}

func (*responseClient) CloseIdleConnections() {}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (failingBody) Close() error {
	return nil
}

func TestTransportBuildsSignedAppRequest(t *testing.T) {
	transport := &transport{host: "https://api.example", deviceUUID: "device-1"}
	request, err := transport.newRequest(
		context.Background(),
		"/api/v2/search",
		url.Values{"q": {"SSIS-589"}},
		"zh-TW",
		1784134914,
	)
	if err != nil {
		t.Fatal(err)
	}

	query := request.URL.Query()
	if query.Get("q") != "SSIS-589" || query.Get("device_uuid") != "device-1" {
		t.Fatalf("query = %v", query)
	}
	if query.Get("app_version") != appVersion || query.Get("platform") != "android" {
		t.Fatalf("public params = %v", query)
	}
	if got := request.Header.Get("jdsignature"); got != signature(1784134914) {
		t.Fatalf("jdsignature = %q", got)
	}
	if got := request.Header.Get("user-agent"); got != userAgent {
		t.Fatalf("user-agent = %q", got)
	}
}

func TestTransportClassifiesResponseReadFailureAsNetworkError(t *testing.T) {
	transport := &transport{
		host:       "https://api.example",
		deviceUUID: "device-1",
		client: &responseClient{response: &http.Response{
			StatusCode: 200,
			Body:       failingBody{},
		}},
	}

	err := transport.getJSON(context.Background(), "/api/v2/search", nil, defaultLanguage, nil)
	var network *networkError
	if !errors.As(err, &network) || !strings.Contains(err.Error(), "read JavDB response") {
		t.Fatalf("error = %v, want networkError", err)
	}
}

var _ io.ReadCloser = failingBody{}
