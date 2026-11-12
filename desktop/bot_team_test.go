package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/session"
)

func teamBotTestApp(t *testing.T) *App {
	a := newManualSessionTestApp(t)
	botModelFixture(t, a)
	t.Cleanup(func() {
		_ = a.stopBotChannels()
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	return a
}

func TestBotMemberRoutingUsesSharedMemberWithoutBroadcast(t *testing.T) {
	a := teamBotTestApp(t)
	created := installBotAdapterFixture(a)
	member, err := a.SaveTeamAgent(SaveTeamAgentRequest{RequestID: "bot-test-create-member", Settings: TeamAgentSettings{Name: "Channel member", InitialModel: "bot-models/second", Permission: "read-only", Folder: t.TempDir(), Preset: &TeamPresetDefinition{ID: "channel", Name: "Channel", Body: "MEMBER_PERSONA", IncludeRuntimeContext: true, RestrictTools: true, Tools: []string{}}}})
	if err != nil || member.Outcome != "applied" {
		t.Fatalf("member creation %s %v", member.Outcome, err)
	}
	inputs := []BotConnectionInput{}
	for _, id := range []string{"wechat", "feishu"} {
		input := BotConnectionInput{Config: botchannels.Config{ID: id, Name: id, Provider: "weixin", Mode: "member", MemberID: member.TargetID, ChatID: "peer", SenderID: "peer"}, Secret: id + "-secret", Connect: true}
		if err := a.SaveBotConnection(input); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, input)
	}
	first, second := nextBotAdapter(t, created), nextBotAdapter(t, created)
	// Independent workers can become ready in either order. Route assertions
	// must use the configured channel identity, never goroutine scheduling.
	if first.connectionID == "feishu" {
		first, second = second, first
	}
	if first.connectionID != "wechat" || second.connectionID != "feishu" {
		t.Fatal("unexpected adapter identities")
	}
	r1, r2 := onlyBotRun(t, a, "wechat"), onlyBotRun(t, a, "feishu")
	first.inbound <- botchannels.Incoming{ID: "one", ChatID: "peer", SenderID: "peer", Text: "hello"}
	select {
	case text := <-first.replies:
		if text != "ok" {
			t.Fatal(text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("origin channel lost reply")
	}
	bound, err := a.prepareBotMember(t.Context(), member.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first channel turn settled", func() bool { return !controllerHasActiveRuntimeWork(bound.ctrl) })
	if len(second.replies) != 0 {
		t.Fatal("reply broadcast to other channel")
	}
	if seat := bound.ctrl.BoundTeamSeat(); seat == nil {
		t.Fatal("channel used an ordinary controller")
	}
	if bound.ctrl.ModelRef() != "bot-models/second" {
		t.Fatal("channel ignored member model")
	}
	// An ordinary Desktop turn in the same member session must stay on Desktop.
	if err := a.submitBotTurn(t.Context(), bound, "desktop input", "desktop-private-input"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "desktop turn settled", func() bool { return !controllerHasActiveRuntimeWork(bound.ctrl) })
	if len(first.replies) != 0 || len(second.replies) != 0 {
		t.Fatal("Desktop reply leaked into a messaging channel")
	}
	second.inbound <- botchannels.Incoming{ID: "two", ChatID: "peer", SenderID: "peer", Text: "hello again"}
	select {
	case text := <-second.replies:
		if text != "ok" {
			t.Fatal(text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("second channel lost own reply")
	}
	waitFor(t, "second channel turn settled", func() bool { return !controllerHasActiveRuntimeWork(bound.ctrl) })
	if len(first.replies) != 0 {
		t.Fatal("second reply leaked to first channel")
	}
	// Exact prompt route: another channel cannot approve or stop this turn.
	r1.mu.Lock()
	r1.routes["owned"] = bound.config.SessionID
	r1.sourceOwners["owned"] = bound.ctrl
	r1.mu.Unlock()
	e := event.Event{Kind: event.ApprovalRequest, SessionID: bound.config.SessionID, SubmissionID: "owned", TurnID: "turn", RuntimeEpoch: "epoch", Approval: event.Approval{ID: "prompt", Tool: "bash", Subject: "fixture"}}
	a.publishBotEvent(bound.sink, e)
	select {
	case <-first.replies:
	case <-time.After(10 * time.Second):
		t.Fatal("origin approval absent")
	}
	if len(second.replies) != 0 {
		t.Fatal("approval leaked to unrelated channel")
	}
	r2.mu.Lock()
	_, leaked := r2.prompts["prompt"]
	r2.mu.Unlock()
	if leaked {
		t.Fatal("approval authority crossed channel")
	}
	// Per-member binding is permanent once saved, and no model overrides survive.
	bad := inputs[0]
	bad.MemberID = "a-1111111111111111"
	if a.SaveBotConnection(bad) == nil {
		t.Fatal("unknown member accepted")
	}
	view, err := a.ListBotChannels()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range view.Connections {
		if c.MemberID != member.TargetID || c.MemberName != "Channel member" {
			t.Fatal("member projection missing")
		}
	}
	blob, _ := json.Marshal(view)
	if strings.Contains(string(blob), "wechat-secret") {
		t.Fatal("credential leaked")
	}
}

func TestBotMemberMigrationPreservesHistoryAndIsIdempotent(t *testing.T) {
	a := teamBotTestApp(t)
	shared, _ := manualAgentPresetTab(t, a, "default")
	bound := &botChannelRun{config: botchannels.Saved{Config: botchannels.Config{SessionID: shared.SessionID}}, ctrl: shared.Ctrl.(*control.Controller), sink: shared.sink, tabID: shared.ID, generation: shared.SessionGeneration}
	if err := a.submitBotTurn(t.Context(), bound, "old input", "old-bot-task"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "legacy turn completed", func() bool { return !controllerHasActiveRuntimeWork(bound.ctrl) })
	old := botchannels.Saved{Config: botchannels.Config{ID: "legacy", Name: "Legacy Bot", Provider: "weixin", Mode: "follow", SessionID: shared.SessionID, ChatID: "peer", SenderID: "peer"}, Secret: "migration-secret", Enabled: true}
	if err := a.botStore().Edit(func([]botchannels.Saved) ([]botchannels.Saved, error) { return []botchannels.Saved{old}, nil }); err != nil {
		t.Fatal(err)
	}
	a.botChannels.op.Lock()
	err := a.migrateBotMembersLocked()
	a.botChannels.op.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := a.botStore().Read()
	if err != nil {
		t.Fatal(err)
	}
	c := entries[0]
	if c.MemberID == "" || c.Mode != "member" || c.LegacySessionID != shared.SessionID || c.Secret != old.Secret || !c.Enabled {
		t.Fatal("legacy configuration lost")
	}
	m, err := botTeamMember(c.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 1 || m.Parts[0].SessionPath != sessionRoute(shared.SessionID) {
		t.Fatal("original history reference lost")
	}
	prepared, err := a.prepareBotMember(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.config.SessionID == shared.SessionID {
		t.Fatal("migration hijacked original session")
	}
	snapshot, err := a.desktopSessionService("").Query().Snapshot(t.Context(), session.SessionRef{HostID: localDesktopHostID, SessionID: prepared.config.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, msg := range snapshot.Projection.Messages {
		if strings.Contains(msg.Content, "old input") {
			found = true
		}
	}
	if !found {
		t.Fatal("member did not inherit completed history")
	}
	if _, err := a.desktopSessionService("").Query().Stat(t.Context(), session.SessionRef{HostID: localDesktopHostID, SessionID: shared.SessionID}); err != nil {
		t.Fatal("original history destroyed")
	}
	// Simulate a crash after roster commit but before channel commit.
	if err := a.botStore().Edit(func([]botchannels.Saved) ([]botchannels.Saved, error) { return []botchannels.Saved{old}, nil }); err != nil {
		t.Fatal(err)
	}
	a.botChannels.op.Lock()
	err = a.migrateBotMembersLocked()
	a.botChannels.op.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	agents, err := a.botMembers()
	if err != nil || len(agents) != 1 {
		t.Fatalf("duplicate migration %v %v", agents, err)
	}
	entries, _ = a.botStore().Read()
	if entries[0].MemberID != c.MemberID {
		t.Fatal("migration changed member identity")
	}
}
