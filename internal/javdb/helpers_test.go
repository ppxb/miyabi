package javdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/time/rate"
)

type fixtureCall struct {
	path   string
	params url.Values
}

type fixtureTransport struct {
	responses map[string][]byte
	calls     []fixtureCall
}

func (transport *fixtureTransport) getJSON(
	_ context.Context,
	path string,
	params url.Values,
	destination any,
) error {
	transport.calls = append(transport.calls, fixtureCall{path: path, params: params})
	body, ok := transport.responses[path]
	if !ok {
		return fmt.Errorf("missing fixture for %s", path)
	}
	return decodeEnvelope(body, destination)
}

func (*fixtureTransport) closeIdleConnections() {}

func fixtureFile(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func clientWithTransport(transport jsonTransport) *Client {
	client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
	client.current.Store(&routeState{transport: transport})
	return client
}

var _ = domain.Media{}
