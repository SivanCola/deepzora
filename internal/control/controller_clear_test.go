package control

import (
	"errors"
	"strings"
	"testing"

	"deepzora/internal/agent"
	"deepzora/internal/event"
	"deepzora/internal/session"
)

func TestClearSessionRefusesWhileTurnRuns(t *testing.T) {
	exec := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, event.Discard)
	c := newOwnedTestController(t, Options{Executor: exec})
	c.mu.Lock()
	c.turns.phase = session.RuntimeRunning
	c.mu.Unlock()

	err := c.ClearSession()
	if err == nil || !errors.Is(err, errTurnRunningRotation) || !strings.Contains(err.Error(), "cannot clear while a turn is running") {
		t.Fatalf("ClearSession while running = %v, want turn-running error", err)
	}
}
