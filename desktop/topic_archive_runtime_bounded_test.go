package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"deepzora/internal/identitylock"
	"deepzora/internal/session"
)

func TestArchiveRuntimeMutationOutlastsBriefBackgroundHolders(t *testing.T) {
	a := &App{}
	a.runtimeRebuildMu.Lock()
	held := true
	defer func() {
		if held {
			a.runtimeRebuildMu.Unlock()
		}
	}()
	if release, ok := a.tryLockRuntimeMutation("probe"); ok {
		release()
		t.Fatal("fixture did not hold the runtime mutation lock")
	}
	// Model several busy observations, then release between two attempts.
	// A synthetic clock makes retry ordering independent of scheduler slices.
	now := time.Unix(0, 0)
	pauses := 0
	release, ok := a.tryLockRuntimeMutationWithClock("archive session", func() time.Time { return now }, func(wait time.Duration) {
		pauses++
		now = now.Add(wait)
		if pauses == 4 {
			a.runtimeRebuildMu.Unlock()
			held = false
		}
	})
	if !ok {
		t.Fatal("archive did not retry after the background holder released")
	}
	release()
	if pauses != 4 {
		t.Fatalf("retry pauses = %d, want 4", pauses)
	}
}

func TestArchiveRuntimeMutationStillRefusesAPersistentHolder(t *testing.T) {
	a := &App{}
	a.runtimeAdmissionMu.RLock()
	defer a.runtimeAdmissionMu.RUnlock()
	now := time.Unix(0, 0)
	elapsed := time.Duration(0)
	nextWait := 5 * time.Millisecond
	release, ok := a.tryLockRuntimeMutationWithClock("archive session", func() time.Time { return now }, func(wait time.Duration) {
		if wait != nextWait {
			t.Fatalf("retry wait = %v, want %v", wait, nextWait)
		}
		// Failed admission must release the rebuild lock before waiting.
		if !a.runtimeRebuildMu.TryLock() {
			t.Fatal("archive retained runtimeRebuildMu during backoff")
		}
		a.runtimeRebuildMu.Unlock()
		elapsed += wait
		now = now.Add(wait)
		nextWait = min(2*wait, 200*time.Millisecond)
	})
	if ok {
		release()
		t.Fatal("archive acquired the lock while a turn held admission")
	}
	if elapsed >= runtimeMutationUserWait || elapsed+nextWait < runtimeMutationUserWait {
		t.Fatalf("retry budget = %v, next wait = %v, limit = %v", elapsed, nextWait, runtimeMutationUserWait)
	}
	// The failed attempt must leave both locks free for the next owner.
	if !a.runtimeRebuildMu.TryLock() {
		t.Fatal("refused archive leaked runtimeRebuildMu")
	}
	a.runtimeRebuildMu.Unlock()
}

func TestSessionBusyCauseKeepsTheHolderIdentity(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errTopicArchiveBusy, "runtime_mutation"},
		{errTopicHasActiveWork, "active_work"},
		{fmt.Errorf("maintenance: %w", session.ErrWriterOwned), "writer_owned"},
		{fmt.Errorf("source: %w", identitylock.ErrHeld), "source_lock"},
	} {
		if got := sessionBusyCause(tc.err); got != tc.want {
			t.Errorf("sessionBusyCause(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// holdRuntimeMutationBriefly keeps runtimeRebuildMu for 100ms, longer than a
// single TryLock tolerates and well inside the bounded user wait.
func holdRuntimeMutationBriefly(t *testing.T, a *App) {
	t.Helper()
	a.runtimeRebuildMu.Lock()
	timer := time.AfterFunc(100*time.Millisecond, a.runtimeRebuildMu.Unlock)
	t.Cleanup(func() { timer.Stop() })
}

func TestTopicRemovalEntryPointsOutlastBriefRuntimeHolders(t *testing.T) {
	t.Run("archive topic", func(t *testing.T) {
		a := &App{}
		holdRuntimeMutationBriefly(t, a)
		if err := a.archiveCompatibleTopic(""); errors.Is(err, errTopicArchiveBusy) {
			t.Fatal("archive gave up at the runtime lock")
		}
	})
	t.Run("remove topic", func(t *testing.T) {
		a := NewApp()
		a.ctx = t.Context()
		installNoopRuntimeEvents(a)
		defer a.closeSessionServices()
		holdRuntimeMutationBriefly(t, a)
		out, _ := a.RemoveTopic(TopicRemovalRequest{OperationID: "op", ExpectedToken: "token", Target: TopicRemovalTarget{WorkspaceID: "ws", TopicID: "topic"}})
		if out.ErrorCode == "busy" {
			t.Fatal("remove topic gave up at the runtime lock")
		}
	})
}
