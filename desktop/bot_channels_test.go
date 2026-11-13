package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/agent"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/session"
	"deepzora/internal/tool"
)

func TestBotConfigurationNeverReturnsSecrets(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	input := BotConnectionInput{Config: botchannels.Config{ID: "test", Name: "Bot", Provider: "telegram", SessionID: "s", ChatID: "c", SenderID: "u"}, Secret: "secret-do-not-return"}
	if err := app.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	input.Secret = ""
	input.Name = "Renamed"
	if err := app.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	view, err := app.ListBotChannels()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(view)
	if strings.Contains(string(data), "secret-do-not-return") || len(view.Connections) != 1 || !view.Connections[0].HasSecret || view.Connections[0].Status != "stopped" {
		t.Fatalf("bad view: %s", data)
	}
	saved, _ := app.botStore().Read()
	if saved[0].Secret != "secret-do-not-return" {
		t.Fatal("blank edit cleared credential")
	}
	if err := app.ConnectBotChannel(input.ID); err == nil {
		t.Fatal("connected without bound runtime")
	}
	if err := app.DeleteBotConnection(input.ID); err != nil {
		t.Fatal(err)
	}
	view, _ = app.ListBotChannels()
	if len(view.Connections) != 0 {
		t.Fatal("delete failed")
	}
}

func TestWeixinCachedLoginIdentityAndSaveDefaults(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	app.botChannels.logins = map[string]*botLoginAttempt{"login": {secret: "private-test-token", userID: "scanner@im.wechat", expires: time.Now().Add(time.Minute)}}
	view, err := app.PollWeixinBotLogin("login")
	if err != nil || view.UserID != "scanner@im.wechat" || view.Status != "success" {
		t.Fatalf("cached login view: %+v %v", view, err)
	}
	b, _ := json.Marshal(view)
	if strings.Contains(string(b), "private-test-token") {
		t.Fatal("credential returned to UI")
	}
	input := BotConnectionInput{Config: botchannels.Config{ID: "weixin", Name: "Weixin", Provider: "weixin", SessionID: "session"}, LoginID: "login"}
	if err := app.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	entries, _ := app.botStore().Read()
	if entries[0].ChatID != view.UserID || entries[0].SenderID != view.UserID {
		t.Fatal("missing IDs not filled from scanner")
	}
	input.ChatID = "manual-chat"
	input.SenderID = "manual-sender"
	if err := app.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	entries, _ = app.botStore().Read()
	if entries[0].ChatID != "manual-chat" || entries[0].SenderID != "manual-sender" {
		t.Fatal("manual access was overwritten")
	}
	app.botChannels.logins["missing"] = &botLoginAttempt{secret: "private-test-token", expires: time.Now().Add(time.Minute)}
	input.LoginID = "missing"
	input.ChatID = ""
	input.SenderID = ""
	if err := app.SaveBotConnection(input); err == nil {
		t.Fatal("login with missing identity granted public access")
	}
}

func TestFeishuCachedLoginAndSaveKeepsSecretServerSide(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	app.botChannels.logins = map[string]*botLoginAttempt{"login": {provider: "lark", secret: "private-app-secret", appID: "cli_fixture", userID: "ou_scanner", expires: time.Now().Add(time.Minute)}}
	view, err := app.PollFeishuBotLogin("login")
	if err != nil || view.UserID != "ou_scanner" || view.ChatID != "openid:ou_scanner" || view.AppID != "cli_fixture" {
		t.Fatalf("login projection: %+v %v", view, err)
	}
	b, _ := json.Marshal(view)
	if strings.Contains(string(b), "private-app-secret") {
		t.Fatal("secret exposed")
	}
	input := BotConnectionInput{Config: botchannels.Config{ID: "lark", Name: "Lark", Provider: "lark", SessionID: "session"}, LoginID: "login"}
	if err := app.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	entries, _ := app.botStore().Read()
	if entries[0].AppID != "cli_fixture" || entries[0].ChatID != view.ChatID || entries[0].Secret != "private-app-secret" {
		t.Fatal("registration not persisted")
	}
	input.Provider = "feishu"
	if app.SaveBotConnection(input) == nil {
		t.Fatal("cross-provider credential accepted")
	}
	app.botChannels.logins["login"].expires = time.Now().Add(-time.Second)
	expired, err := app.PollFeishuBotLogin("login")
	if err != nil || expired.Status != "expired" || expired.AppID != "" {
		t.Fatal("expired registration exposed credentials")
	}
}

func botTestRuntime(t *testing.T) (*App, *botChannelRun) {
	t.Helper()
	isolateDesktopUserDirs(t)
	dir := t.TempDir()
	service, err := session.NewService("local", session.NewFilesystemPersistence(dir))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	sink := &tabEventSink{tabID: "tab", app: app, runtimeEpoch: "epoch"}
	sink.runtimeEvents.emit = func(context.Context, string, ...any) {}
	runner := agent.New(workspaceContextProvider{}, tool.NewRegistry(), agent.NewSession("system"), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{Runner: runner, Executor: runner, SystemPrompt: "system", SessionDir: dir, SessionService: service, ExclusiveSession: true, Sink: sink})
	if _, err := ctrl.BindFreshSession(t.Context(), "bot-session"); err != nil {
		t.Fatal(err)
	}
	tab := &WorkspaceTab{ID: "tab", SessionID: "bot-session", Scope: "global", Ready: true, Ctrl: ctrl, sink: sink}
	app.mu.Lock()
	app.tabs = map[string]*WorkspaceTab{"tab": tab}
	app.tabOrder = []string{"tab"}
	app.activeTabID = "tab"
	app.mu.Unlock()
	_, cancel := context.WithCancel(t.Context())
	r := &botChannelRun{config: botchannels.Saved{Config: botchannels.Config{ID: "test", Provider: "telegram", SessionID: "bot-session", ChatID: "allowed-chat", SenderID: "allowed-sender"}}, ctrl: ctrl, sink: sink, tabID: "tab", cancel: cancel, done: make(chan struct{}), events: make(chan event.Event, 128), output: make(chan string, 64), prompts: map[string]control.PromptIdentity{}, receipts: &botchannels.Receipts{Path: filepath.Join(dir, "receipts.json")}}
	app.botChannels.mu.Lock()
	app.botChannels.runs = map[string]*botChannelRun{"test": r}
	app.botChannels.mu.Unlock()
	t.Cleanup(func() { cancel(); ctrl.Close(); <-ctrl.Closed(); _ = service.CloseAll(context.Background()) })
	return app, r
}
func TestBotRejectsUnauthorizedMessagesAndStaleBinding(t *testing.T) {
	app, r := botTestRuntime(t)
	for _, m := range []botchannels.Incoming{{ID: "1", ChatID: "wrong", SenderID: "allowed-sender", Text: "hello"}, {ID: "2", ChatID: "allowed-chat", SenderID: "wrong", Text: "/stop"}} {
		if err := app.receiveBotMessage(t.Context(), r, m); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.output) != 0 || r.ctrl.RuntimeStateSnapshot().Running {
		t.Fatal("unauthorized message had side effects")
	}
	app.mu.Lock()
	app.tabs["tab"].SessionID = "new-session"
	app.mu.Unlock()
	if err := app.receiveBotMessage(t.Context(), r, botchannels.Incoming{ID: "3", ChatID: "allowed-chat", SenderID: "allowed-sender", Text: "hello"}); err == nil {
		t.Fatal("stale session accepted")
	}
}
func TestBotTextUsesExistingSessionAndDeduplicates(t *testing.T) {
	app := newManualSessionTestApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	if _, err := app.SaveProviderWithKey(ProviderView{Name: "bot-fixture", Kind: "openai", BaseURL: server.URL, Models: []string{"test"}, NoProxy: true}, "fixture-secret"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetDefaultModel("bot-fixture/test"); err != nil {
		t.Fatal(err)
	}
	tab, _ := manualAgentPresetTab(t, app, "default")
	_, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &botChannelRun{config: botchannels.Saved{Config: botchannels.Config{ID: "test", Provider: "telegram", SessionID: tab.SessionID, ChatID: "allowed-chat", SenderID: "allowed-sender"}}, ctrl: tab.Ctrl.(*control.Controller), sink: tab.sink, tabID: tab.ID, generation: tab.SessionGeneration, cancel: cancel, done: make(chan struct{}), events: make(chan event.Event, 128), output: make(chan string, 64), prompts: map[string]control.PromptIdentity{}, receipts: &botchannels.Receipts{Path: filepath.Join(t.TempDir(), "receipts.json")}}
	app.botChannels.mu.Lock()
	app.botChannels.runs = map[string]*botChannelRun{"test": r}
	app.botChannels.mu.Unlock()
	m := botchannels.Incoming{ID: "1", ChatID: "allowed-chat", SenderID: "allowed-sender", Text: "hello"}
	if err := app.receiveBotMessage(t.Context(), r, m); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	messages := 0
	for {
		select {
		case e := <-r.events:
			if e.Kind == event.Message {
				messages++
				app.forwardBotEvent(r, e)
			}
			if e.Kind == event.TurnDone {
				goto done
			}
		case <-deadline:
			t.Fatalf("no completed turn; output=%v", drainBotOutput(r))
		}
	}
done:
	if messages != 1 {
		t.Fatalf("messages=%d output=%v", messages, drainBotOutput(r))
	}
	if err := app.receiveBotMessage(t.Context(), r, m); err != nil {
		t.Fatal(err)
	}
	// A duplicate is consumed by the persisted receipt without a new turn.
	select {
	case e := <-r.events:
		t.Fatalf("duplicate event: %v", e.Kind)
	default:
	}
	if got := drainBotOutput(r); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("replies=%v", got)
	}
}
func drainBotOutput(r *botChannelRun) []string {
	var out []string
	for {
		select {
		case s := <-r.output:
			out = append(out, s)
		default:
			return out
		}
	}
}

func TestBotOldCommandsNotReplayedAndEventsStayScoped(t *testing.T) {
	app, r := botTestRuntime(t)
	m := botchannels.Incoming{ID: "command", ChatID: "allowed-chat", SenderID: "allowed-sender", Text: "/help"}
	if err := app.receiveBotMessage(t.Context(), r, m); err != nil {
		t.Fatal(err)
	}
	_ = drainBotOutput(r)
	if err := app.receiveBotMessage(t.Context(), r, m); err != nil {
		t.Fatal(err)
	}
	if len(r.output) != 0 {
		t.Fatal("command replayed")
	}
	app.publishBotEvent(r.sink, event.Event{Kind: event.Message, SessionID: "other", Text: "private"})
	if len(r.events) != 0 {
		t.Fatal("cross-session event forwarded")
	}
	app.publishBotEvent(r.sink, event.Event{Kind: event.Message, SessionID: r.config.SessionID, Text: "shared"})
	if len(r.events) != 1 {
		t.Fatal("bound event missing")
	}
}

type blockedBotAdapter struct {
	ready, sendStarted chan struct{}
	once               sync.Once
}

func (b *blockedBotAdapter) Run(ctx context.Context, _ func(botchannels.Incoming) error, status func(string)) error {
	status("connected")
	close(b.ready)
	<-ctx.Done()
	return ctx.Err()
}
func (b *blockedBotAdapter) Send(ctx context.Context, _ string) error {
	b.once.Do(func() { close(b.sendStarted) })
	<-ctx.Done()
	return ctx.Err()
}
func TestBotDisconnectJoinsReceiveAndBlockedSend(t *testing.T) {
	app, r := botTestRuntime(t)
	ctx, cancel := context.WithCancel(t.Context())
	r.cancel = cancel
	adapter := &blockedBotAdapter{ready: make(chan struct{}), sendStarted: make(chan struct{})}
	go func() { defer close(r.done); app.runBotChannel(ctx, r, adapter) }()
	<-adapter.ready
	r.enqueue("reply")
	<-adapter.sendStarted
	if err := app.DisconnectBotChannel("test"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.done:
	default:
		t.Fatal("workers survived disconnect")
	}
	app.botChannels.mu.Lock()
	remaining := len(app.botChannels.runs)
	app.botChannels.mu.Unlock()
	if remaining != 0 {
		t.Fatal("disconnected runtime retained")
	}
}
func TestBotApprovalKeepsExactIdentityAndRejectsStaleReply(t *testing.T) {
	app, r := botTestRuntime(t)
	app.forwardBotEvent(r, event.Event{Kind: event.ApprovalRequest, SessionID: r.config.SessionID, TurnID: "old-turn", RuntimeEpoch: "old-epoch", Approval: event.Approval{ID: "approval-1", Tool: "bash", Subject: "example"}})
	identity := r.prompts["approval-1"]
	if identity.TurnID != "old-turn" || identity.RuntimeEpoch != "old-epoch" {
		t.Fatal("approval identity lost")
	}
	_ = drainBotOutput(r)
	if err := app.receiveBotMessage(t.Context(), r, botchannels.Incoming{ID: "answer", ChatID: "allowed-chat", SenderID: "allowed-sender", Text: "/approve approval-1"}); err != nil {
		t.Fatal(err)
	}
	replies := drainBotOutput(r)
	if len(replies) != 1 || !strings.Contains(replies[0], "no longer available") {
		t.Fatalf("stale approval accepted: %v", replies)
	}
	app.forwardBotEvent(r, event.Event{Kind: event.PromptAnswered, ItemID: "approval-1"})
	if len(r.prompts) != 0 {
		t.Fatal("settled approval retained")
	}
}
func TestBotConfigCannotChangeWhileAnotherConsumerOwnsAccount(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	input := BotConnectionInput{Config: botchannels.Config{ID: "test", Name: "Bot", Provider: "telegram", SessionID: "s", ChatID: "c", SenderID: "u"}, Secret: "fixture-secret"}
	if err := app.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	saved, _ := app.botStore().Read()
	unlock, err := app.lockBotAccount(saved[0])
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	input.ChatID = "different"
	if err := app.SaveBotConnection(input); err == nil {
		t.Fatal("changed live account binding")
	}
	if err := app.DeleteBotConnection(input.ID); err == nil {
		t.Fatal("deleted live account binding")
	}
}
