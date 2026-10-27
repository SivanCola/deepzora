package main

import (
	"context"
	"testing"
	"time"
)

// The sidebar reloads topic pages only on project-tree:changed-v2. A new
// session or title change reaches the catalog as a metadata sync request, so
// that request must publish the event; otherwise rows stay stale until restart.
func TestSessionCatalogMetadataSyncPublishesProjectTreeChangedV2(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	app.ctx = context.Background()
	events := make(chan string, 64)
	app.runtimeEvents.emit = func(_ context.Context, name string, _ ...any) {
		select {
		case events <- name:
		default:
		}
	}
	waitForInitialCatalogReconcile(t, app)
	drain := time.After(200 * time.Millisecond)
	for drained := false; !drained; {
		select {
		case <-events:
		case <-drain:
			drained = true
		}
	}

	app.emitProjectTreeMetadataChanged()

	deadline := time.After(sessionCatalogTestDeadline)
	for {
		select {
		case name := <-events:
			if name == "project-tree:changed-v2" {
				return
			}
		case <-deadline:
			t.Fatal("metadata sync request did not publish project-tree:changed-v2")
		}
	}
}
