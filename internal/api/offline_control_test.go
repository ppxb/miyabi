package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

type controlledOffline struct {
	OfflineManager
	cancelled []int
	switched  []int
}

func (m *controlledOffline) Cancel(_ context.Context, id int) (domain.OfflineSubmission, error) {
	m.cancelled = append(m.cancelled, id)
	return domain.OfflineSubmission{TaskID: id, Status: "cancelled"}, nil
}
func (m *controlledOffline) TryNext(_ context.Context, id int) (domain.OfflineSubmission, error) {
	m.switched = append(m.switched, id)
	return domain.OfflineSubmission{TaskID: id, Status: "running", DownloadState: "submitting"}, nil
}

func TestOfflineControlsTargetOneValidatedTask(t *testing.T) {
	m := &controlledOffline{}
	router := gin.New()
	router.Use(errorMiddleware(slog.Default()))
	router.POST("/offline/:id/cancel", offlineControlHandler(m, false))
	router.POST("/offline/:id/next", offlineControlHandler(m, true))
	for _, tc := range []struct {
		path   string
		status int
	}{{"/offline/13/cancel", http.StatusAccepted}, {"/offline/21/next", http.StatusAccepted}, {"/offline/0/cancel", http.StatusBadRequest}, {"/offline/no/next", http.StatusBadRequest}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	if len(m.cancelled) != 1 || m.cancelled[0] != 13 || len(m.switched) != 1 || m.switched[0] != 21 {
		t.Fatalf("wrong task controls: %+v", m)
	}
}
