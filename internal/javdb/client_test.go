package javdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/netx"
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
	any,
) error {
	transport.calls++
	return transport.err
}

func (*stubTransport) closeIdleConnections() {}

func TestRouteProbeResultsRemainIndependentOfActiveSelection(t *testing.T) {
	var responseCode atomic.Int32
	responseCode.Store(http.StatusServiceUnavailable)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/startup" {
			t.Errorf("unexpected probe path %s", r.URL.Path)
		}
		w.WriteHeader(int(responseCode.Load()))
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer server.Close()
	originalHosts := bootstrapHosts
	bootstrapHosts = []string{server.URL}
	defer func() { bootstrapHosts = originalHosts }()
	client, err := New(Options{
		DeviceUUID: "00000000-0000-4000-8000-000000000000",
		CachedHost: server.URL, CachedLatency: 125 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	before := client.current.Load()
	initial, _ := client.Route()
	if _, err := client.reselect(t.Context()); err == nil {
		t.Fatal("failed probe was accepted")
	}
	status, active := client.Route()
	if !active || client.current.Load() != before || status.Host != server.URL || status.Latency != 125*time.Millisecond {
		t.Fatalf("failed probe replaced active selection: %+v", status)
	}
	if len(status.Candidates) != 1 || status.Candidates[0].Status != RouteUnavailable {
		t.Fatalf("failed measurements not exposed: %+v", status.Candidates)
	}
	if initial.Candidates[0].Status != RouteAvailable {
		t.Fatal("new probe mutated previously returned measurements")
	}
	measurements := client.lastProbe.Load()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.reselect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled selection = %v", err)
	}
	if client.lastProbe.Load() != measurements || client.current.Load() != before {
		t.Fatal("cancelled selection changed route state")
	}
	responseCode.Store(http.StatusOK)
	selected, err := client.reselect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if selected.Host != server.URL || len(selected.Candidates) != 1 || selected.Candidates[0].Status != RouteAvailable {
		t.Fatalf("selection response lost route or candidates: %+v", selected)
	}
	status, active = client.Route()
	if !active || client.current.Load() == before || status.Host != selected.Host || !slices.Equal(status.Candidates, selected.Candidates) {
		t.Fatalf("installed route disagrees with response: %+v", status)
	}
}

func TestClientReselectKeepsBootstrapWithMalformedBackupData(t *testing.T) {
	invalidDomains := startupWithBackupDomains(t, []byte(`{"apiDomains":[42]}`))
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "wrong field type", data: `42`},
		{name: "null field", data: `null`},
		{name: "invalid ciphertext", data: `"AA=="`},
		{name: "invalid domains", data: string(invalidDomains.BackupDomainsData)},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/startup" {
					t.Errorf("unexpected probe path %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"success":true,"data":{"backup_domains_data":` + test.data + `}}`))
			}))
			defer server.Close()
			originalHosts := bootstrapHosts
			bootstrapHosts = []string{server.URL}
			defer func() { bootstrapHosts = originalHosts }()
			client, err := New(Options{DeviceUUID: "00000000-0000-4000-8000-000000000000"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			result, err := client.reselect(t.Context())
			if err != nil || result.Host != server.URL || len(result.Candidates) != 1 || result.Candidates[0].Status != RouteAvailable {
				t.Fatalf("healthy bootstrap rejected: result = %+v, error = %v", result, err)
			}
		})
	}
}

func TestClientReusesCachedRouteWithoutSelecting(t *testing.T) {
	client, err := New(Options{
		DeviceUUID: "00000000-0000-4000-8000-000000000000",
		CachedHost: "https://cached.example", CachedLatency: 125 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	status, active := client.Route()
	if !active || status.Host != "https://cached.example" || status.Latency != 125*time.Millisecond {
		t.Fatalf("restored route = %#v, active = %t", status, active)
	}

	cached := client.current.Load()
	cached.transport.closeIdleConnections()
	transport := &stubTransport{}
	client.current.Store(&routeState{transport: transport, host: cached.host, latency: cached.latency})
	selections := 0
	client.selector = func(context.Context, routeSelection) (*routeState, error) {
		selections++
		return nil, errors.New("cached route must be reused without probing")
	}
	if err := client.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := client.getJSON(t.Context(), "/test", nil, nil); err != nil {
		t.Fatal(err)
	}
	if selections != 0 || transport.calls != 1 {
		t.Fatalf("selections = %d, business requests = %d", selections, transport.calls)
	}
}

func TestClientReselectStillMeasuresAllRoutesWithCache(t *testing.T) {
	client, err := New(Options{
		DeviceUUID: "00000000-0000-4000-8000-000000000000",
		CachedHost: "https://cached.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	selections := 0
	client.selector = func(_ context.Context, options routeSelection) (*routeState, error) {
		selections++
		if !options.full || !slices.Contains(options.hosts, "https://cached.example") {
			return nil, errors.New("reselection must measure all routes, including the cached host")
		}
		return &routeState{host: "https://selected.example"}, nil
	}
	status, err := client.reselect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if selections != 1 || status.Host != "https://selected.example" {
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
	client.selector = func(_ context.Context, options routeSelection) (*routeState, error) {
		selections++
		if options.full {
			return nil, errors.New("connection failure must use quick route recovery")
		}
		client.current.Store(replacementState)
		return replacementState, nil
	}

	if err := client.getJSON(t.Context(), "/test", nil, nil); err != nil {
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
		{name: "json", err: errors.New("decode JavDB data")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &stubTransport{err: test.err}
			client := &Client{limiter: rate.NewLimiter(rate.Inf, 1)}
			client.current.Store(&routeState{transport: transport})
			client.selector = func(context.Context, routeSelection) (*routeState, error) {
				t.Fatal("route selection must not run")
				return nil, nil
			}

			err := client.getJSON(t.Context(), "/test", nil, nil)
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
		client.selector = func(context.Context, routeSelection) (*routeState, error) {
			selections++
			state := &routeState{transport: replacement}
			client.current.Store(state)
			return state, nil
		}
		if err := client.getJSON(t.Context(), "/test", nil, nil); !errors.Is(err, replacement.err) {
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
		client.selector = func(ctx context.Context, options routeSelection) (*routeState, error) {
			if !options.full {
				t.Error("initial selection must measure every candidate")
			}
			selections.Add(1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-finish:
				return &routeState{host: "https://selected.example"}, nil
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
			client.selector = func(ctx context.Context, _ routeSelection) (*routeState, error) {
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

func TestClientRebuildsAndReselectsWhenProxyChanges(t *testing.T) {
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(Options{
		DeviceUUID: "00000000-0000-4000-8000-000000000000",
		CachedHost: "https://cached.example", Proxy: proxy,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	before := client.current.Load()
	selections := make(chan routeSelection, 1)
	client.selector = func(_ context.Context, options routeSelection) (*routeState, error) {
		current := client.current.Load()
		if current == before {
			return nil, errors.New("selection started before rebuilding the transport")
		}
		next := *current
		next.host = "https://selected.example"
		client.current.Store(&next)
		selections <- options
		return &next, nil
	}

	if err := proxy.Update(netx.ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	select {
	case selection := <-selections:
		if !selection.full || !slices.Contains(selection.hosts, before.host) || client.current.Load().host != "https://selected.example" {
			t.Fatalf("proxy change did not reselect routes: %+v", selection)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("routes were not reselected after proxy change")
	}
}
