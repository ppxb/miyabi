package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/monitor"
)

type subscriptionStub struct {
	SubscriptionManager
	targetsCalledKind string
	actorFeedID       int
	actorFeedPage     int
	actorFeedLimit    int
	actorFeedCalls    int
}

func (s *subscriptionStub) Targets(_ context.Context, kind string) ([]monitor.TargetItem, error) {
	s.targetsCalledKind = kind
	return []monitor.TargetItem{
		{ID: 1, Kind: "movie", TargetID: "m1", Status: subscription.StatusWaiting},
		{ID: 2, Kind: "actor", TargetID: "a1", Status: subscription.StatusActive},
	}, nil
}

func (s *subscriptionStub) ActorFeed(_ context.Context, actorID, page, limit int) ([]monitor.Item, error) {
	s.actorFeedCalls++
	s.actorFeedID = actorID
	s.actorFeedPage = page
	s.actorFeedLimit = limit
	return []monitor.Item{
		{ID: 3, Kind: "movie", TargetID: "m-spawned", Title: "Spawned"},
	}, nil
}

func (s *subscriptionStub) UpdateConfig(_ context.Context, cfg monitor.Config) (monitor.Config, error) {
	if cfg.CheckTime == "" {
		cfg.CheckTime = "00:00"
	}
	return cfg, nil
}

func TestSubscriptionTargetsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionStub{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/subscriptions/targets?kind=movie", nil)

	subscriptionTargetsHandler(stub)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if stub.targetsCalledKind != "movie" {
		t.Fatalf("expected kind 'movie', got '%s'", stub.targetsCalledKind)
	}
	var items []monitor.TargetItem
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestSubscriptionActorFeedEndpoint(t *testing.T) {
	for _, tc := range []struct {
		path            string
		status          int
		id, page, limit int
	}{
		{"/feed", http.StatusOK, 0, 1, 50},
		{"/feed?page=2&limit=20", http.StatusOK, 0, 2, 20},
		{"/42/feed", http.StatusOK, 42, 1, 50},
		{"/42/feed?page=3&limit=100", http.StatusOK, 42, 3, 100},
		{"/feed?kind=unused", http.StatusOK, 0, 1, 50},
		{"/42/feed?kind=unused", http.StatusOK, 42, 1, 50},
		{"/abc/feed", http.StatusBadRequest, 0, 0, 0},
		{"/0/feed", http.StatusBadRequest, 0, 0, 0},
		{"/-1/feed", http.StatusBadRequest, 0, 0, 0},
		{"/999999999999999999999999/feed", http.StatusBadRequest, 0, 0, 0},
		{"/feed?page=0", http.StatusBadRequest, 0, 0, 0},
		{"/42/feed?page=abc", http.StatusBadRequest, 0, 0, 0},
		{"/feed?limit=0", http.StatusBadRequest, 0, 0, 0},
		{"/42/feed?limit=101", http.StatusBadRequest, 0, 0, 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			stub := &subscriptionStub{}
			router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Monitor: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/subscriptions/actors"+tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status=%d, want %d: %s", response.Code, tc.status, response.Body)
			}
			if tc.status != http.StatusOK {
				if stub.actorFeedCalls != 0 {
					t.Fatal("invalid request reached actor feed service")
				}
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error == "" {
					t.Fatalf("missing error response: %s (%v)", response.Body, err)
				}
				return
			}
			if stub.actorFeedCalls != 1 || stub.actorFeedID != tc.id || stub.actorFeedPage != tc.page || stub.actorFeedLimit != tc.limit {
				t.Fatalf("unexpected feed call: %+v", stub)
			}
			var items []monitor.Item
			if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil || len(items) != 1 || items[0].ID != 3 {
				t.Fatalf("unexpected feed response: %s (%v)", response.Body, err)
			}
		})
	}
}

func TestSubscriptionSettingsUpdateHandler_ReturnsNormalizedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionStub{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/settings/subscription", bytes.NewReader([]byte(`{"check_time":""}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	subscriptionSettingsUpdateHandler(stub)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res monitor.Config
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.CheckTime != "00:00" {
		t.Fatalf("expected normalized check_time '00:00', got %q", res.CheckTime)
	}
}
