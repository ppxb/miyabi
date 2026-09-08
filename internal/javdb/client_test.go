package javdb

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

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

func (*stubTransport) closeIdleConnections() {}

func TestClientReusesCachedRouteWithoutSelecting(t *testing.T) {
	for _, test := range []struct {
		name   string
		manual bool
	}{
		{name: "automatic"},
		{name: "manual", manual: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := New(Options{
				DeviceUUID: "00000000-0000-4000-8000-000000000000",
				CachedHost: "https://cached.example", CachedLatency: 125 * time.Millisecond,
				ManualRoute: test.manual,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			status, active := client.Route()
			if !active || status.Host != "https://cached.example" || status.Manual != test.manual || status.Latency != 125*time.Millisecond {
				t.Fatalf("restored route = %#v, active = %t", status, active)
			}

			cached := client.current.Load()
			cached.transport.closeIdleConnections()
			transport := &stubTransport{}
			client.current.Store(&routeState{transport: transport, status: cached.status})
			selections := 0
			client.selectRoute = func(context.Context, routeSelection) (*routeState, error) {
				selections++
				return nil, errors.New("cached route must be reused without probing")
			}
			if err := client.Initialize(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := client.getJSON(t.Context(), "/test", nil, defaultLanguage, nil); err != nil {
				t.Fatal(err)
			}
			if selections != 0 || transport.calls != 1 {
				t.Fatalf("selections = %d, business requests = %d", selections, transport.calls)
			}
		})
	}
}

func TestClientReselectStillMeasuresAllRoutesWithCache(t *testing.T) {
	client, err := New(Options{
		DeviceUUID: "00000000-0000-4000-8000-000000000000",
		CachedHost: "https://cached.example", ManualRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	selections := 0
	client.selectRoute = func(_ context.Context, options routeSelection) (*routeState, error) {
		selections++
		if !options.full || !slices.Contains(options.hosts, "https://cached.example") {
			return nil, errors.New("manual reselect must measure all routes, including the cached host")
		}
		return &routeState{status: RouteStatus{Host: "https://selected.example"}}, nil
	}
	status, err := client.Reselect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if selections != 1 || status.Host != "https://selected.example" || status.Manual {
		t.Fatalf("selections = %d, selected route = %#v", selections, status)
	}
}

func TestClientReplaysOnceAfterRouteFailure(t *testing.T) {
	failedTransport := &stubTransport{err: &networkError{err: errors.New("connection reset")}}
	replacementTransport := &stubTransport{}
	failedState := &routeState{transport: failedTransport}
	replacementState := &routeState{transport: replacementTransport}

	selections := 0
	client := &Client{limiter: rate.NewLimiter(rate.Inf, 1), routeContext: t.Context(), options: Options{Timeout: time.Second}}
	client.current.Store(failedState)
	client.selectRoute = func(_ context.Context, options routeSelection) (*routeState, error) {
		selections++
		if options.full {
			return nil, errors.New("connection failure must use quick route recovery")
		}
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
		{name: "http 401", err: &HTTPError{StatusCode: 401}},
		{name: "http 429", err: &HTTPError{StatusCode: 429}},
		{name: "json", err: errors.New("decode JavDB data")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &stubTransport{err: test.err}
			client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
			client.current.Store(&routeState{transport: transport})
			client.selectRoute = func(context.Context, routeSelection) (*routeState, error) {
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

func TestClientReselectsForGatewayErrorsButOnlyReplaysOnce(t *testing.T) {
	for _, code := range []int{502, 503, 504} {
		failed := &stubTransport{err: &HTTPError{StatusCode: code}}
		replacement := &stubTransport{err: &HTTPError{StatusCode: code}}
		client := &Client{limiter: rate.NewLimiter(rate.Inf, 1), routeContext: t.Context(), options: Options{Timeout: time.Second}}
		client.current.Store(&routeState{transport: failed})
		selections := 0
		client.selectRoute = func(context.Context, routeSelection) (*routeState, error) {
			selections++
			state := &routeState{transport: replacement}
			client.current.Store(state)
			return state, nil
		}
		if err := client.getJSON(t.Context(), "/test", nil, defaultLanguage, nil); !errors.Is(err, replacement.err) {
			t.Fatalf("HTTP %d error = %v", code, err)
		}
		if failed.calls != 1 || replacement.calls != 1 || selections != 1 {
			t.Fatalf("HTTP %d: calls = %d + %d, selections = %d", code, failed.calls, replacement.calls, selections)
		}
	}
}

func TestClientRouteSelectionOutlivesCanceledCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, err := New(Options{DeviceUUID: "00000000-0000-4000-8000-000000000000"})
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		var selections atomic.Int32
		finish := make(chan struct{})
		client.selectRoute = func(ctx context.Context, options routeSelection) (*routeState, error) {
			if !options.full {
				t.Error("initial selection must measure every candidate")
			}
			selections.Add(1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-finish:
				return &routeState{status: RouteStatus{Host: "https://selected.example"}}, nil
			}
		}
		first, cancel := context.WithCancel(t.Context())
		firstResult := make(chan error, 1)
		go func() { firstResult <- client.Initialize(first) }()
		synctest.Wait()
		secondResult := make(chan error, 1)
		go func() { secondResult <- client.Initialize(t.Context()) }()
		synctest.Wait()
		cancel()
		if err := <-firstResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("first error = %v", err)
		}
		close(finish)
		if err := <-secondResult; err != nil {
			t.Fatalf("second error = %v", err)
		}
		if selections.Load() != 1 {
			t.Fatalf("selections = %d", selections.Load())
		}
	})
}

func TestClientRouteSelectionStopsOnTimeoutAndClose(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			client, err := New(Options{DeviceUUID: "00000000-0000-4000-8000-000000000000", Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.selectRoute = func(ctx context.Context, _ routeSelection) (*routeState, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			result := make(chan error, 1)
			go func() { result <- client.Initialize(t.Context()) }()
			synctest.Wait()
			want := context.DeadlineExceeded
			if shutdown {
				client.Close()
				want = context.Canceled
			}
			if err := <-result; !errors.Is(err, want) {
				t.Fatalf("shutdown %t: error = %v", shutdown, err)
			}
		})
	}
}
