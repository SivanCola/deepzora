package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"deepzora/internal/agent"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/session"
)

// newCanonicalSessionTUI builds an exclusive-session TUI over a fresh
// sessions-v4 catalog sharing the CLI's cached session service, plus a second,
// closed ("held") identity tests can resume.
func newCanonicalSessionTUI(t *testing.T) (*chatTUI, *control.Controller, *session.Service, session.SessionRef) {
	t.Helper()
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	service := testCLISessionService(t, sessionDir)
	if service == nil {
		t.Fatal("session service unavailable for test workspace")
	}
	ctrl := newOwnedTestController(t, control.Options{
		Executor:   agent.New(nil, nil, agent.NewSession("system"), agent.Options{}, event.Discard),
		SessionDir: sessionDir, SessionService: service, ExclusiveSession: true,
	})
	if _, err := ctrl.BindFreshSession(t.Context(), "fresh-cli"); err != nil {
		t.Fatal(err)
	}
	held, err := service.Create(t.Context(), session.CreateOptions{SessionID: "held"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(t.Context(), held.Ref()); err != nil {
		t.Fatal(err)
	}
	m := newTestChatTUI()
	m.ctrl = ctrl
	t.Cleanup(func() {
		ctrl.Close()
		_ = service.CloseAll(context.Background())
	})
	return &m, ctrl, service, held.Ref()
}
