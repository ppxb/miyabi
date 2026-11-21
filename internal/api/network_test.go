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

	"github.com/ppxb/miyabi/internal/netx"
)

type networkStub struct {
	config netx.ProxyConfig
}

func (stub *networkStub) Config() netx.ProxyConfig {
	return stub.config
}

func (stub *networkStub) UpdateNetwork(_ context.Context, config netx.ProxyConfig) error {
	stub.config = config
	return nil
}

func TestNetworkEndpointsReadAndWrite(t *testing.T) {
	stub := &networkStub{config: netx.ProxyConfig{Enabled: true, URL: "http://user:secret@127.0.0.1:7890"}}
	router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Network: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

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
