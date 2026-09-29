package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/network"
	"github.com/ppxb/miyabi/internal/netx"
)

type networkStub struct {
	config     netx.ProxyConfig
	tested     netx.ProxyConfig
	testResult network.TestResult
}

func (stub *networkStub) Config() netx.ProxyConfig {
	return stub.config
}

func (stub *networkStub) UpdateNetwork(_ context.Context, config netx.ProxyConfig) error {
	stub.config = config
	return nil
}

func (stub *networkStub) TestNetwork(_ context.Context, config netx.ProxyConfig) (network.TestResult, error) {
	stub.tested = config
	return stub.testResult, nil
}

func TestNetworkEndpointsReadAndWrite(t *testing.T) {
	stub := &networkStub{config: netx.ProxyConfig{Enabled: true, URL: "http://user:secret@127.0.0.1:7890"}}
	router := NewRouter(Dependencies{Network: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/settings/network", nil))
	if get.Code != http.StatusOK || get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET status=%d headers=%v body=%s", get.Code, get.Header(), get.Body)
	}
	var response netx.ProxyConfig
	if err := json.Unmarshal(get.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response != stub.config {
		t.Fatalf("GET returned %+v, want %+v", response, stub.config)
	}

	put := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(`{"enabled":false,"url":"https://127.0.0.1:7891"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(put, request)
	if put.Code != http.StatusOK || stub.config != (netx.ProxyConfig{Enabled: false, URL: "https://127.0.0.1:7891"}) {
		t.Fatalf("PUT status=%d config=%+v body=%s", put.Code, stub.config, put.Body)
	}
}

func TestNetworkTestEndpoint(t *testing.T) {
	stub := &networkStub{
		config: netx.ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"},
		testResult: network.TestResult{
			JavDB:  network.ProbeResult{Available: true, LatencyMS: 120},
			JavBus: network.ProbeResult{Available: false, Error: "timeout"},
		},
	}
	router := NewRouter(Dependencies{Network: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	post := httptest.NewRecorder()
	router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/settings/network/test", nil))
	if post.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", post.Code, post.Body)
	}

	var result network.TestResult

	if err := json.Unmarshal(post.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.JavDB.Available || result.JavDB.LatencyMS != 120 {
		t.Fatalf("unexpected JavDB result: %+v", result.JavDB)
	}
	if result.JavBus.Available || result.JavBus.Error != "timeout" {
		t.Fatalf("unexpected JavBus result: %+v", result.JavBus)
	}
	if stub.tested != stub.config {
		t.Fatalf("empty body tested %+v, want saved config %+v", stub.tested, stub.config)
	}

	candidate := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/network/test", strings.NewReader(`{"enabled":true,"url":"socks5://127.0.0.1:1080"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(candidate, request)
	if candidate.Code != http.StatusOK || stub.tested != (netx.ProxyConfig{Enabled: true, URL: "socks5://127.0.0.1:1080"}) {
		t.Fatalf("candidate status=%d tested=%+v", candidate.Code, stub.tested)
	}
}
