package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/monitor"
)

type subscriptionStub struct {
	SubscriptionManager
	targetsCalledKind string
	actorFeedID       int
	actorFeedPage     int
	actorFeedLimit    int
}

func (s *subscriptionStub) Targets(_ context.Context, kind string) ([]monitor.TargetItem, error) {
	s.targetsCalledKind = kind
	return []monitor.TargetItem{
		{ID: 1, Kind: "movie", TargetID: "m1", Status: monitor.StatusWaiting},
		{ID: 2, Kind: "actor", TargetID: "a1", Status: monitor.StatusActive},
	}, nil
}

func (s *subscriptionStub) ActorFeed(_ context.Context, actorID, page, limit int) ([]monitor.Item, error) {
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

func TestSubscriptionActorFeedHandlerAllSpawned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionStub{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/subscriptions/actors/feed?page=2&limit=20", nil)

	subscriptionActorFeedHandler(stub)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if stub.actorFeedID != 0 {
		t.Fatalf("expected actor ID 0, got %d", stub.actorFeedID)
	}
	if stub.actorFeedPage != 2 || stub.actorFeedLimit != 20 {
		t.Fatalf("expected page 2 limit 20, got page %d limit %d", stub.actorFeedPage, stub.actorFeedLimit)
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
