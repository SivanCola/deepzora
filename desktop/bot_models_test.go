package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/session"
)

func botModelFixture(t *testing.T, a *App) <-chan string {
	t.Helper()
	requests := make(chan string, 32)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("model credential not reused")
		}
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests <- body.Model
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(s.Close)
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "bot-models", DisplayName: func() *string { s := "Bot models"; return &s }(), Kind: "openai", BaseURL: s.URL, Models: []string{"first", "second"}, NoProxy: true}, "fixture-secret"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("bot-models/first"); err != nil {
		t.Fatal(err)
	}
	return requests
}

func TestBotDedicatedModelSessionIsolationAndReopen(t *testing.T) {
	a := newManualSessionTestApp(t)
	requests := botModelFixture(t, a)
	shared, _ := manualAgentPresetTab(t, a, "default")
	a.mu.Lock()
	a.activeTabID = shared.ID
	a.mu.Unlock()
	input := BotConnectionInput{Config: botchannels.Config{ID: "dedicated", Name: "Weixin", Provider: "weixin", Mode: "dedicated", ModelRef: "bot-models/second", WorkspaceID: "global", SessionID: shared.SessionID, ChatID: "peer", SenderID: "peer"}, Secret: "private-bot-token"}
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	entries, _ := a.botStore().Read()
	c := entries[0].Config
	if c.SessionID == shared.SessionID || c.SessionID != botDedicatedSession(c) {
		t.Fatal("caller hijacked dedicated session identity")
	}
	if a.metaForSession(c.SessionID) != nil {
		t.Fatal("saving started a runtime")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := a.prepareDedicatedBotSession(ctx, c); err != nil {
		t.Fatal(err)
	}
	r := &botChannelRun{config: entries[0]}
	if err := a.bindBotRun(r); err != nil {
		t.Fatal(err)
	}
	if r.ctrl.ModelRef() != "bot-models/second" || r.ctrl == shared.Ctrl {
		t.Fatal("bot did not receive an independent model/runtime")
	}
	if shared.Ctrl.(*control.Controller).ModelRef() != "bot-models/first" || a.activeTabID != shared.ID {
		t.Fatal("bot changed shared conversation or focus")
	}
	first := r.ctrl
	if err := a.prepareDedicatedBotSession(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := a.bindBotRun(r); err != nil || r.ctrl != first {
		t.Fatalf("reconnect replaced runtime: %v", err)
	}
	view, err := a.ListBotChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Connections) != 1 || view.Connections[0].ActualModel != "bot-models/second" {
		t.Fatal("actual model not exposed")
	}
	for _, s := range view.Sessions {
		if s.ID == c.SessionID {
			t.Fatal("dedicated session offered to other bots")
		}
	}
	data, _ := json.Marshal(view)
	if strings.Contains(string(data), "fixture-secret") || strings.Contains(string(data), "private-bot-token") {
		t.Fatal("model/bot credential leaked")
	}
	if meta := a.metaForSession(c.SessionID); meta == nil || !strings.HasPrefix(meta.TopicTitle, "Bot · ") {
		t.Fatal("dedicated conversation not named for its bot")
	}
	// Real admission and provider execution must honor the bot's selection,
	// even while the active Desktop conversation uses a different model.
	r.cancel = cancel
	r.done = make(chan struct{})
	r.events = make(chan event.Event, 128)
	r.output = make(chan string, 64)
	r.prompts = map[string]control.PromptIdentity{}
	r.receipts = &botchannels.Receipts{Path: filepath.Join(t.TempDir(), "receipts.json")}
	a.botChannels.mu.Lock()
	a.botChannels.runs = map[string]*botChannelRun{c.ID: r}
	a.botChannels.mu.Unlock()
	defer func() { a.botChannels.mu.Lock(); delete(a.botChannels.runs, c.ID); a.botChannels.mu.Unlock() }()
	if err := a.receiveBotMessage(ctx, r, botchannels.Incoming{ID: "message", ChatID: "peer", SenderID: "peer", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	completed := false
	for !completed {
		select {
		case e := <-r.events:
			a.forwardBotEvent(r, e)
			if e.Kind == event.TurnDone {
				if e.Err != nil {
					t.Fatal(e.Err)
				}
				completed = true
			}
		case <-ctx.Done():
			t.Fatal("bot model turn did not finish")
		}
	}
	select {
	case model := <-requests:
		if model != "second" {
			t.Fatalf("wrong provider model: %s", model)
		}
	default:
		t.Fatal("no provider request")
	}
	if replies := drainBotOutput(r); len(replies) != 1 || replies[0] != "ok" {
		t.Fatalf("reply missing: %v", replies)
	}
	sharedSnapshot, err := a.desktopSessionService("").Query().Snapshot(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: shared.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range sharedSnapshot.Projection.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			t.Fatal("bot contaminated shared transcript")
		}
	}
	a.botChannels.mu.Lock()
	delete(a.botChannels.runs, c.ID)
	a.botChannels.mu.Unlock()
	waitFor(t, "bot turn settled before cold reopen", func() bool { return !controllerHasActiveRuntimeWork(r.ctrl) })
	closedController := r.ctrl
	if err := a.CloseTab(r.tabID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closedController.Closed():
	case <-ctx.Done():
		t.Fatal("old bot runtime did not close")
	}
	if err := a.prepareDedicatedBotSession(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := a.bindBotRun(r); err != nil {
		t.Fatal(err)
	}
	if r.config.SessionID != c.SessionID || r.ctrl.ModelRef() != "bot-models/second" {
		t.Fatal("reopen lost dedicated binding")
	}
	if r.ctrl == closedController {
		t.Fatal("cold reopen reused closed controller")
	}
	restored, err := a.desktopSessionService("").Query().Snapshot(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: c.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	answers := 0
	for _, m := range restored.Projection.Messages {
		if m.Role == "user" && m.Content == "hello" {
			users++
		}
		if m.Role == "assistant" && m.Content == "ok" {
			answers++
		}
	}
	if users != 1 || answers != 1 {
		roles := []string{}
		for _, m := range restored.Projection.Messages {
			roles = append(roles, string(m.Role))
		}
		t.Fatalf("reopening lost bot conversation history: roles=%v", roles)
	}
	if err := a.desktopSessionService("").SetModel(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: c.SessionID}, "bot-models/first", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.prepareDedicatedBotSession(ctx, c); err == nil {
		t.Fatal("reconnect silently overwrote a model changed in Desktop")
	}
	// Switching to another model preserves the first session and chooses a new
	// isolated identity, never altering the shared conversation.
	input.ModelRef = "bot-models/first"
	input.Secret = ""
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	changed, _ := a.botStore().Read()
	if changed[0].SessionID == c.SessionID {
		t.Fatal("model change reused old context")
	}
	if _, err := a.desktopSessionService("").Query().Stat(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: c.SessionID}); err != nil {
		t.Fatal("old history lost", err)
	}
	input.Mode = "follow"
	input.SessionID = shared.SessionID
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	follow, _ := a.botStore().Read()
	if follow[0].SessionID != shared.SessionID || follow[0].ModelRef != "" || follow[0].WorkspaceID != "" {
		t.Fatal("follow mode retained model override")
	}
}

func TestBotDedicatedValidationAndProbe(t *testing.T) {
	a := newManualSessionTestApp(t)
	botModelFixture(t, a)
	base := BotConnectionInput{Config: botchannels.Config{ID: "bot", Name: "Bot", Provider: "telegram", Mode: "dedicated", WorkspaceID: "global", ModelRef: "bot-models/first", ChatID: "peer", SenderID: "peer"}, Secret: "fixture"}
	for _, mutate := range []func(*BotConnectionInput){
		func(c *BotConnectionInput) { c.Mode = "unknown" },
		func(c *BotConnectionInput) { c.ModelRef = "unknown/model" },
		func(c *BotConnectionInput) { c.WorkspaceID = "missing" },
		func(c *BotConnectionInput) { c.Mode = "follow"; c.SessionID = "desktop-bot-someone-else" },
	} {
		bad := base
		mutate(&bad)
		if a.SaveBotConnection(bad) == nil {
			t.Fatal("invalid bot settings accepted")
		}
	}
	other := base.Config
	other.ID = "other"
	if botDedicatedSession(other) == botDedicatedSession(base.Config) {
		t.Fatal("bots share session identity")
	}
	if err := a.TestBotModel(base.ModelRef); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 0 {
		t.Fatal("probe created a conversation")
	}
	if err := a.prepareDedicatedBotSession(t.Context(), base.Config); err == nil {
		t.Fatal("unowned session identity accepted")
	}
}

func TestBotLegacyModeAndDedicatedValidation(t *testing.T) {
	legacy := botchannels.Config{ID: "legacy", Name: "Legacy", Provider: "telegram", SessionID: "shared", ChatID: "chat", SenderID: "sender"}
	if err := botchannels.Validate(legacy); err != nil {
		t.Fatal(err)
	}
	legacy.Mode = "dedicated"
	legacy.ModelRef = "p/m"
	legacy.WorkspaceID = "global"
	legacy.SessionID = ""
	if err := botchannels.Validate(legacy); err != nil {
		t.Fatal(err)
	}
	legacy.ModelRef = ""
	if botchannels.Validate(legacy) == nil {
		t.Fatal("missing dedicated model accepted")
	}
}
