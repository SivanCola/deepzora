package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/event"
	"deepzora/internal/session"
)

type lifecycleBotAdapter struct {
	inbound        chan botchannels.Incoming
	replies        chan string
	ready, stopped chan struct{}
	sendErr        error
}

func (b *lifecycleBotAdapter) Run(ctx context.Context, receive func(botchannels.Incoming) error, status func(string)) error {
	defer close(b.stopped)
	status("connected")
	close(b.ready)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-b.inbound:
			if err := receive(m); err != nil {
				return err
			}
		}
	}
}
func (b *lifecycleBotAdapter) Send(ctx context.Context, text string) error {
	if b.sendErr != nil {
		return b.sendErr
	}
	select {
	case b.replies <- text:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func installBotAdapterFixture(a *App) <-chan *lifecycleBotAdapter {
	created := make(chan *lifecycleBotAdapter, 16)
	a.botChannels.adapterFactory = func(botchannels.Saved, *http.Client) botchannels.Adapter {
		b := &lifecycleBotAdapter{inbound: make(chan botchannels.Incoming, 8), replies: make(chan string, 16), ready: make(chan struct{}), stopped: make(chan struct{})}
		created <- b
		return b
	}
	return created
}
func nextBotAdapter(t *testing.T, created <-chan *lifecycleBotAdapter) *lifecycleBotAdapter {
	t.Helper()
	select {
	case b := <-created:
		select {
		case <-b.ready:
			return b
		case <-time.After(10 * time.Second):
			t.Fatal("bot did not become ready")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("bot did not start")
	}
	return nil
}
func onlyBotRun(t *testing.T, a *App, id string) *botChannelRun {
	t.Helper()
	a.botChannels.mu.Lock()
	defer a.botChannels.mu.Unlock()
	r := a.botChannels.runs[id]
	if r == nil {
		t.Fatal("missing bot runtime")
	}
	return r
}

func TestBotSaveEnableReconnectAndRestartIntent(t *testing.T) {
	a := newManualSessionTestApp(t)
	botModelFixture(t, a)
	created := installBotAdapterFixture(a)
	t.Cleanup(func() { _ = a.stopBotChannels() })
	input := BotConnectionInput{Config: botchannels.Config{ID: "enabled", Name: "Bot", Provider: "weixin", Mode: "dedicated", ModelRef: "bot-models/second", WorkspaceID: "global", ChatID: "peer", SenderID: "peer"}, Secret: "fixture"}
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	entries, _ := a.botStore().Read()
	if entries[0].Enabled || len(created) != 0 {
		t.Fatal("save-only activated bot")
	}
	input.Connect = true
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	first := nextBotAdapter(t, created)
	input.Connect = false
	input.Secret = ""
	input.Name = "Edited"
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	second := nextBotAdapter(t, created)
	select {
	case <-first.stopped:
	default:
		t.Fatal("old adapter survived edit")
	}
	if first == second {
		t.Fatal("edit did not reconnect")
	}
	entries, _ = a.botStore().Read()
	if !entries[0].Enabled || entries[0].Name != "Edited" || entries[0].Secret != "fixture" {
		t.Fatal("edit lost enabled state or credential")
	}
	if err := a.DisconnectBotChannel(input.ID); err != nil {
		t.Fatal(err)
	}
	a.restoreEnabledBotChannels()
	entries, _ = a.botStore().Read()
	if entries[0].Enabled || len(created) != 0 {
		t.Fatal("manual disconnect restored unexpectedly")
	}
	if err := a.ConnectBotChannel(input.ID); err != nil {
		t.Fatal(err)
	}
	nextBotAdapter(t, created)
	if err := a.stopBotChannels(); err != nil {
		t.Fatal(err)
	}
	entries, _ = a.botStore().Read()
	if !entries[0].Enabled {
		t.Fatal("shutdown disabled saved bot")
	}
	// New process state: use the same saved configuration and canonical files.
	for _, tab := range a.tabs {
		if tab.Ctrl != nil {
			tab.Ctrl.Close()
			if closed, ok := tab.Ctrl.(interface{ Closed() <-chan struct{} }); ok {
				<-closed.Closed()
			}
		}
	}
	_ = a.stopManualCreations()
	a.closeSessionServices()
	_ = a.sessionUI.Close()
	b := NewApp()
	b.ctx = t.Context()
	installNoopRuntimeEvents(b)
	restored := installBotAdapterFixture(b)
	t.Cleanup(func() {
		_ = b.stopBotChannels()
		_ = b.stopManualCreations()
		for _, tab := range b.tabs {
			if tab.Ctrl != nil {
				tab.Ctrl.Close()
				if closed, ok := tab.Ctrl.(interface{ Closed() <-chan struct{} }); ok {
					<-closed.Closed()
				}
			}
		}
		b.closeSessionServices()
		if b.sessionUI != nil {
			_ = b.sessionUI.Close()
		}
	})
	b.tabsRestored = make(chan struct{})
	done := make(chan struct{})
	go func() { b.restoreEnabledBotChannels(); close(done) }()
	select {
	case <-done:
		t.Fatal("restoration did not wait for desktop startup")
	case <-time.After(20 * time.Millisecond):
	}
	b.markTabsRestored()
	<-done
	nextBotAdapter(t, restored)
	r := onlyBotRun(t, b, input.ID)
	if r.config.SessionID != entries[0].SessionID || r.ctrl.ModelRef() != input.ModelRef {
		t.Fatal("restart retargeted the bot")
	}
	b.restoreEnabledBotChannels()
	if len(restored) != 0 {
		t.Fatal("duplicate startup replaced an already running bot")
	}
}

func TestBotRuntimeTabHandoffPreservesInputAndReply(t *testing.T) {
	a := newManualSessionTestApp(t)
	requests := botModelFixture(t, a)
	ordinary, _ := manualAgentPresetTab(t, a, "default")
	ordinaryRef := session.SessionRef{HostID: localDesktopHostID, SessionID: ordinary.SessionID}
	a.mu.Lock()
	a.activeTabID = ordinary.ID
	a.mu.Unlock()
	created := installBotAdapterFixture(a)
	t.Cleanup(func() { _ = a.stopBotChannels() })
	input := BotConnectionInput{Config: botchannels.Config{ID: "moving", Name: "Bot", Provider: "weixin", Mode: "dedicated", ModelRef: "bot-models/second", WorkspaceID: "global", ChatID: "peer", SenderID: "peer"}, Secret: "fixture", Connect: true}
	if err := a.SaveBotConnection(input); err != nil {
		t.Fatal(err)
	}
	b := nextBotAdapter(t, created)
	r := onlyBotRun(t, a, input.ID)
	botRef := session.SessionRef{HostID: localDesktopHostID, SessionID: r.config.SessionID}
	if _, err := a.OpenSession(botRef); err != nil {
		t.Fatal(err)
	}
	if _, err := a.OpenSession(ordinaryRef); err != nil {
		t.Fatal(err)
	}
	if !a.botBindingValid(r) {
		t.Fatal("same session runtime was invalidated by tab handoff")
	}
	b.inbound <- botchannels.Incoming{ID: "first", ChatID: "peer", SenderID: "peer", Text: "hello"}
	select {
	case answer := <-b.replies:
		if answer != "ok" {
			t.Fatalf("reply after navigation: %s", answer)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reply lost after tab handoff")
	}
	select {
	case model := <-requests:
		if model != "second" {
			t.Fatal("wrong model after handoff")
		}
	default:
		t.Fatal("model was not called")
	}
	waitFor(t, "provider send acknowledgement", func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.received == 1 && r.sent == 1 && r.lastSentAt != ""
	})
	a.publishBotEvent(r.sink, event.Event{Kind: event.Message, SessionID: "unrelated", Text: "must not forward"})
	if len(b.replies) != 0 {
		t.Fatal("forwarded another session")
	}
	if _, err := a.OpenSession(botRef); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	target := a.botRuntimeTabLocked(r)
	if target == nil {
		a.mu.Unlock()
		t.Fatal("reattach lost bot runtime")
	}
	target.SessionID = "unrelated"
	a.mu.Unlock()
	r.enqueue("must not send")
	select {
	case <-b.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("changed session did not stop connection")
	}
	if len(b.replies) != 0 {
		t.Fatal("sent after real session replacement")
	}
}

func TestBotFailedSendDoesNotRecordSuccess(t *testing.T) {
	a, r := botTestRuntime(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r.cancel = cancel
	b := &lifecycleBotAdapter{inbound: make(chan botchannels.Incoming), replies: make(chan string, 1), ready: make(chan struct{}), stopped: make(chan struct{}), sendErr: errors.New("fixture send failure")}
	go func() { defer close(r.done); a.runBotChannel(ctx, r, b) }()
	<-b.ready
	r.enqueue("reply")
	<-r.done
	if r.sent != 0 || r.lastSentAt != "" || r.status != "error" {
		t.Fatal("failed delivery claimed success")
	}
}
