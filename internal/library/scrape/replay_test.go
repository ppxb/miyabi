package scrape

import (
	"encoding/json"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
	"testing"
)

func TestCompletedScrapeReplayNeedsNoExternalServices(t *testing.T) {
	service := &Service{}
	for _, completed := range []bool{true, false} {
		body, err := tasks.EncodePayload(Payload{Completed: completed})
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			err := service.Scrape(t.Context(), tasks.Job{ID: 1, Payload: body})
			if completed && err != nil || !completed && !domain.IsKind(err, domain.KindInvalid) {
				t.Fatalf("replay completed=%t: %v", completed, err)
			}
		}
	}
	if err := service.Scrape(t.Context(), tasks.Job{Payload: json.RawMessage(`{"completed":`)}); err == nil {
		t.Fatal("malformed checkpoint accepted")
	}
}
