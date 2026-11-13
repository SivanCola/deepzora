package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"deepzora/internal/agentpreset"
	"deepzora/internal/session"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func TestDesktopTeamUsesCanonicalBackgroundRuntimeAndSettlesRealInbox(t *testing.T) {
	a := newManualSessionTestApp(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	requests := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "team-fixture", Kind: "openai", BaseURL: server.URL, Models: []string{"test"}, NoProxy: true}, "disposable-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("team-fixture/test"); err != nil {
		t.Fatal(err)
	}
	foreground, _ := manualAgentPresetTab(t, a, "default")
	a.mu.Lock()
	a.activeTabID = foreground.ID
	a.mu.Unlock()
	changed := make(chan struct{}, 32)
	a.teamState.changed = func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	if err := h.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	var chief, ada teamroster.Agent
	store := desktopTeamStore()
	_, err = store.Update(ctx, func(r *teamroster.Roster) error {
		chief = r.AddAgent(teamroster.Agent{Name: "Chief", Main: true, Permission: "read-only"})
		p, err := agentpreset.Parse("copied", "---\ncomplete: true\ninclude-runtime-context: false\ntools: []\n---\nCOPIED_TEAM_PERSONA")
		if err != nil {
			return err
		}
		ada = r.AddAgent(teamroster.Agent{Name: "Ada", Preset: &p, Permission: "read-only"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := h.Ensure(ctx, team.SeatRef{Agent: ada.ID}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	r, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	bound, _ := r.Agent(ada.ID)
	id, ok := parseSessionRoute(bound.SessionPath)
	if !ok {
		t.Fatal("member has no canonical binding")
	}
	a.mu.RLock()
	if a.activeTabID != foreground.ID {
		t.Error("background seat stole foreground selection")
	}
	var runtimeCount int
	var owner *WorkspaceTab
	for _, tab := range a.runtimeTabsLocked() {
		if tab.SessionID == id {
			runtimeCount++
			owner = tab
		}
	}
	a.mu.RUnlock()
	if runtimeCount != 1 || owner == nil {
		t.Fatalf("duplicate or missing runtime: %d", runtimeCount)
	}
	if a.BrowserTeamAgentForTab(owner.ID) != ada.ID {
		t.Fatal("browser owner lost Team identity")
	}
	workspace, err := a.canonicalSessionWorkspace(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: id})
	if err != nil || workspace.Root == "" {
		t.Fatalf("canonical workspace: %+v %v", workspace, err)
	}
	var exchange string
	err = h.WithWriter(ctx, func(ctx context.Context, s *team.Service) error {
		receipt, err := s.MessageAgent(ctx, team.Caller{Agent: chief.ID, Turn: "fixture", Ceiling: "read-only"}, team.MessageRequest{To: ada.ID, Message: "answer briefly", RequestID: "background-fixture"})
		exchange = receipt.Exchange
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-requests:
		if !strings.Contains(body, "COPIED_TEAM_PERSONA") || !strings.Contains(body, "Ada") {
			t.Fatal("member snapshot or identity missing from real request")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for {
		book, err := store.Exchanges()
		if err != nil {
			t.Fatal(err)
		}
		complete := false
		for _, x := range book {
			if x.ID == exchange && x.State == teamroster.ExchangeReplied && x.Returned {
				complete = true
			}
		}
		if complete {
			break
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal("exchange did not settle after the durable reply was consumed")
		}
	}
	// Revealing and closing a chat release the display only, even while idle.
	meta, err := a.openTopicTabWithActivation("project", workspace.Root, "", bound.SessionPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CloseTab(meta.ID); err != nil {
		t.Fatal(err)
	}
	if rt, ok := h.Live(team.SeatRef{Agent: ada.ID}); !ok || rt == nil {
		t.Fatal("close discarded Team seat")
	}
	if pane, err := h.Ensure(ctx, team.SeatRef{Agent: ada.ID}); err != nil || pane == nil {
		t.Fatal(err)
	}
	if err := a.stopTeamHost(); err != nil {
		t.Fatal(err)
	}
	if release, err := store.AcquireLease(); err != nil {
		t.Fatalf("shutdown retained team lease: %v", err)
	} else {
		release()
	}
}
