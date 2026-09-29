package catalogue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/javdb"
)

type routeStateClient struct {
	stubProviderWithMagnets
	status javdb.RouteStatus
	active bool
}

func (c *routeStateClient) Route() (javdb.RouteStatus, bool) { return c.status, c.active }

func TestRouteStateIsIndependentOfPersistence(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved := persistedRoute{Host: "https://old.example", LatencyMS: 125, Manual: true}
	if err := database.SaveSetting(ctx, store.Client, javdbRouteSetting, saved); err != nil {
		t.Fatal(err)
	}
	client := &routeStateClient{
		active: true,
		status: javdb.RouteStatus{
			Host: "https://new.example", Latency: 42 * time.Millisecond,
			Candidates: []javdb.RouteCandidate{{Host: "https://new.example", Latency: 42 * time.Millisecond, Status: javdb.RouteAvailable}},
		},
	}
	service, err := NewWithClients(ctx, store.Client, client, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	assertCurrent := func() {
		t.Helper()
		status := service.Route()
		if !status.Active || status.Host != client.status.Host || status.LatencyMS != 42 || status.Manual || len(status.Candidates) != 1 || status.Candidates[0].LatencyMS != 42 {
			t.Fatalf("current route came from saved state or lost its projection: %+v", status)
		}
	}
	assertCurrent()
	writes := 0
	writeFailure := errors.New("temporary setting write failure")
	store.Client.Setting.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			writes++
			if writes == 1 {
				return nil, writeFailure
			}
			return next.Mutate(ctx, mutation)
		})
	})
	if err := service.persistActiveRoute(ctx); !errors.Is(err, writeFailure) {
		t.Fatalf("write failure: %v", err)
	}
	if service.lastSavedRoute != saved {
		t.Fatal("failed write updated the saved snapshot")
	}
	assertCurrent()
	if err := service.persistActiveRoute(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatalf("expected failed write, retry, then no-op; writes=%d", writes)
	}
	stored, found, err := database.LoadSetting[persistedRoute](ctx, store.Client, javdbRouteSetting)
	if err != nil || !found || stored != (persistedRoute{Host: client.status.Host, LatencyMS: 42}) {
		t.Fatalf("stored route: %+v %v", stored, err)
	}

	// An inactive client may still report probe candidates, but must not borrow
	// the last saved host and manual-selection flag for its current state.
	client.active = false
	client.status = javdb.RouteStatus{Candidates: client.status.Candidates}
	status := service.Route()
	if status.Active || status.Host != "" || status.LatencyMS != 0 || status.Manual || len(status.Candidates) != 1 {
		t.Fatalf("inactive route used stale saved state: %+v", status)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatal("inactive client overwrote the saved route")
	}
}
