package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/event"
)

func TestIncompleteReadIsPausedNotProviderError(t *testing.T) {
	if got := exitBucket(event.Event{Outcome: event.TurnOutcomeIncompleteRead, Err: errors.New("incomplete read")}); got != "incomplete_read" {
		t.Fatalf("exit bucket = %q", got)
	}
}

func TestCleanupRemovesPendingQueueOnly(t *testing.T) {
	home := t.TempDir()
	if err := appendPending(home, pendingPayload{
		Version: "v1.20.0", OS: "linux", Counters: []Counter{{Signal: "turns", Bucket: "count", Count: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	idPath := filepath.Join(home, "cli-telemetry-install-id")
	if err := os.WriteFile(idPath, []byte(strings.Repeat("a", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, pendingDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending directory still exists: %v", err)
	}
	if _, err := os.Stat(idPath); err != nil {
		t.Fatalf("install id should remain stable after opt-out cleanup: %v", err)
	}
}

func TestEnvironmentOptOutRemovesPendingQueue(t *testing.T) {
	clearPolicyEnv(t)
	home := t.TempDir()
	if err := appendPending(home, pendingPayload{
		Version: "v1.20.0", OS: "linux", Counters: []Counter{{Signal: "turns", Bucket: "count", Count: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DO_NOT_TRACK", "1")
	if reporter := Start(Options{Mode: "on", Version: "v1.20.0", HomeDir: home, Interactive: true}); reporter != nil {
		t.Fatal("environment opt-out unexpectedly started telemetry")
	}
	if _, err := os.Stat(filepath.Join(home, pendingDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment opt-out did not remove pending queue: %v", err)
	}
}
