package api

import (
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

func taskEventsHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Subscribe before reading to avoid losing an update between the initial
		// snapshot and stream setup. Reconnection always starts with a snapshot.
		updates, unsubscribe := tasks.Subscribe()
		defer unsubscribe()
		snapshot, err := tasks.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.SSEvent("tasks", snapshot)
		c.SSEvent("changes", taskChangesEvent(tasks))
		c.Writer.Flush()
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		c.Stream(func(io.Writer) bool {
			select {
			case <-c.Request.Context().Done():
				return false
			case <-heartbeat.C:
				c.SSEvent("ping", nil)
			case <-updates:
				snapshot, err := tasks.List(c.Request.Context())
				if err != nil {
					c.Error(err)
					return false
				}
				c.SSEvent("tasks", snapshot)
				c.SSEvent("changes", taskChangesEvent(tasks))
			}
			return true
		})
	}
}

// taskChangesEvent carries the business revisions plus the process instance so
// clients can tell a real change from a restart that reset the counters.
func taskChangesEvent(manager TaskManager) any {
	revisions := manager.Revisions()
	return struct {
		Library  uint64 `json:"library"`
		Offline  uint64 `json:"offline"`
		Monitor  uint64 `json:"monitor"`
		Instance string `json:"instance"`
	}{revisions.Library, revisions.Offline, revisions.Monitor, manager.Instance()}
}
