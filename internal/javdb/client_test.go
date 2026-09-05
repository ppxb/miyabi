package javdb

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"golang.org/x/time/rate"
)

type stubTransport struct {
	err   error
	calls int
}

func (transport *stubTransport) getJSON(
	context.Context,
	string,
	url.Values,
	string,
	any,
) error {
	transport.calls++
	return transport.err
}

func (*stubTransport) getMedia(context.Context, string) (Media, error) {
	return Media{}, errors.New("media is not stubbed")
}

func (*stubTransport) closeIdleConnections() {}

func TestClientReplaysOnceAfterRouteFailure(t *testing.T) {
	failedTransport := &stubTransport{err: &networkError{err: errors.New("connection reset")}}
	replacementTransport := &stubTransport{}
	failedState := &routeState{transport: failedTransport}
	replacementState := &routeState{transport: replacementTransport}

	selections := 0
	client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
	client.current.Store(failedState)
	client.selectRoute = func(context.Context, string) (*routeState, error) {
		selections++
		client.current.Store(replacementState)
		return replacementState, nil
	}

	if err := client.getJSON(t.Context(), "/test", nil, defaultLanguage, nil); err != nil {
		t.Fatal(err)
	}
	if failedTransport.calls != 1 || replacementTransport.calls != 1 || selections != 1 {
		t.Fatalf(
			"failed calls = %d, replacement calls = %d, selections = %d",
			failedTransport.calls,
			replacementTransport.calls,
			selections,
		)
	}
}

func TestClientDoesNotReselectForProtocolOrClientErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "api", err: &APIError{Action: "BadRequest", Message: "invalid"}},
		{name: "http 400", err: &HTTPError{StatusCode: 400}},
		{name: "json", err: errors.New("decode JavDB data")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &stubTransport{err: test.err}
			client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
			client.current.Store(&routeState{transport: transport})
			client.selectRoute = func(context.Context, string) (*routeState, error) {
				t.Fatal("route selection must not run")
				return nil, nil
			}

			err := client.getJSON(t.Context(), "/test", nil, defaultLanguage, nil)
			if !errors.Is(err, test.err) {
				t.Fatalf("error = %v, want %v", err, test.err)
			}
			if transport.calls != 1 {
				t.Fatalf("calls = %d", transport.calls)
			}
		})
	}
}
