package control

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"deepzora/internal/agent"
	"deepzora/internal/event"
	"deepzora/internal/permission"
	"deepzora/internal/provider"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"deepzora/internal/tool"
)

func teamControllerFixture(t *testing.T, runner agent.Runner, executor *agent.Agent) (*Controller, *team.Service, teamroster.Store, string, string) {
	t.Helper()
	store := teamroster.Store{Dir: t.TempDir()}
	svc := team.New(team.Options{Store: store})
	r, err := svc.Bootstrap(t.Context(), "Chief", "Lead")
	if err != nil {
		t.Fatal(err)
	}
	name := "Ada"
	ada, err := svc.UserCreateAgent(t.Context(), team.UserAgentChange{AgentChange: team.AgentChange{Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	c := newCurrentOwnedTestController(t, Options{Runner: runner, Executor: executor,
		Team: TeamOptions{Seat: team.Seat{Service: svc, Ref: team.SeatRef{Agent: ada.ID}}}})
	c.EnableInteractiveApproval()
	c.SetToolApprovalMode(ToolApprovalDangerFullAccess)
	return c, svc, store, r.Agents[0].ID, ada.ID
}

func TestTeamInboxRunsWithHostOriginIdentityAndSettles(t *testing.T) {
	session := agent.NewSession("unchanged system")
	p := &scriptedTurns{turns: [][]provider.Chunk{textTurn("Ada answer")}}
	executor := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	c, _, store, chief, ada := teamControllerFixture(t, executor, executor)
	x, _, err := store.OpenExchange(t.Context(), teamroster.Exchange{From: chief, To: ada, ReplyTo: teamroster.ReplyToUser, Ceiling: "read-only"}, "Investigate")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := c.EnqueueInbox(InboxRequest{Submit: "Investigate", Raw: "Investigate", Source: "team:" + chief,
		Extra: map[string]string{team.ExtraKind: string(team.DeliverMessage), team.ExtraFrom: chief,
			team.ExtraExchange: x.ID, team.ExtraCeiling: "read-only"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RunInboxTurn(t.Context(), receipt.ItemID); err != nil {
		t.Fatal(err)
	}
	book, err := store.Exchanges()
	if err != nil || len(book) != 1 || book[0].State != teamroster.ExchangeReplied || !book[0].Returned {
		t.Fatalf("unsettled exchange: %+v, %v", book, err)
	}
	if c.ToolApprovalMode() != ToolApprovalDangerFullAccess || c.effectiveApprovalMode() != ToolApprovalDangerFullAccess {
		t.Fatal("delegated ceiling changed persistent permission or leaked into next turn")
	}
	found := false
	for _, m := range session.Snapshot() {
		if m.Role == provider.RoleSystem && m.Content != "unchanged system" {
			t.Fatal("team changed system prefix")
		}
		if m.RawContent == "Investigate" {
			found = true
			if m.Origin != provider.MessageOriginHost || !strings.Contains(m.Content, "<agent-identity>") || !strings.Contains(m.Content, "Ada") {
				t.Fatalf("delivery lost host identity: %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("missing durable team input")
	}
}

func TestTeamCeilingPreservesUserChangesAndRestrictsChildren(t *testing.T) {
	c := newOwnedTestController(t, Options{SubagentGate: NewSharedHeadlessGate(permission.Policy{}, ToolApprovalDangerFullAccess)})
	c.SetToolApprovalMode(ToolApprovalDangerFullAccess)
	release := c.holdTeamCeiling(ToolApprovalReadOnly)
	c.approval.setPlanAutoApprove(true)
	if c.approval.preApproved("write_file", "file", nil) {
		t.Fatal("approved-plan bypass widened delegated ceiling")
	}
	if c.ToolApprovalMode() != ToolApprovalDangerFullAccess || c.effectiveApprovalMode() != ToolApprovalReadOnly {
		t.Fatal("wrong effective/persistent mode")
	}
	allow, _, err := c.subagentGate.Check(t.Context(), "write_file", nil, false)
	if err != nil || allow {
		t.Fatalf("child widened ceiling: %v, %v", allow, err)
	}
	c.SetToolApprovalMode(ToolApprovalWorkspaceWrite)
	if c.effectiveApprovalMode() != ToolApprovalReadOnly {
		t.Fatal("persistent update widened running turn")
	}
	release()
	if c.ToolApprovalMode() != ToolApprovalWorkspaceWrite || c.effectiveApprovalMode() != ToolApprovalWorkspaceWrite {
		t.Fatal("scope release lost user's new selection")
	}
}

func TestTeamWorkSettledBeforeRestartNeverReplays(t *testing.T) {
	runner := &fakeTurnRunner{}
	c, _, store, chief, ada := teamControllerFixture(t, runner, nil)
	x, _, err := store.OpenExchange(t.Context(), teamroster.Exchange{From: chief, To: ada, ReplyTo: teamroster.ReplyToUser}, "do not run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetExchangeState(t.Context(), x.ID, teamroster.ExchangeInterrupted, time.Now()); err != nil {
		t.Fatal(err)
	}
	receipt, err := c.EnqueueInbox(InboxRequest{Submit: "do not run", Source: "team:" + chief, Extra: map[string]string{team.ExtraKind: string(team.DeliverMessage), team.ExtraExchange: x.ID}})
	if err != nil {
		t.Fatal(err)
	}
	err = c.RunInboxTurn(t.Context(), receipt.ItemID)
	var refusal tool.Refusal
	if !errors.As(err, &refusal) || refusal.Code != "team.interrupted" || len(runner.inputs) != 0 {
		t.Fatalf("settled work ran: %v, %v", runner.inputs, err)
	}
}

func TestTeamDirectoryRejectsTaskChildIdentity(t *testing.T) {
	c, _, _, _, _ := teamControllerFixture(t, &fakeTurnRunner{}, nil)
	_, err := (teamDirectory{c}).ListAgents(agent.WithSubagentDepth(t.Context(), 1))
	var refusal tool.Refusal
	if !errors.As(err, &refusal) || refusal.Code != "team.child" {
		t.Fatalf("child borrowed parent identity: %v", err)
	}
}

func TestTeamPersistentChangesAlwaysNeedANewHumanAnswer(t *testing.T) {
	requests := make(chan event.Approval, 2)
	c := newOwnedTestController(t, Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			requests <- e.Approval
		}
	})})
	c.SetToolApprovalMode(ToolApprovalDangerFullAccess)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for range 2 {
		done := make(chan error, 1)
		go func() { done <- c.confirmTeamEdit(ctx, "Ada", nil, "Change Ada's instructions") }()
		select {
		case approval := <-requests:
			if !approval.Fresh || approval.Tool != TeamChangeApprovalTool {
				t.Fatal("not a fresh human card")
			}
			c.Approve(approval.ID, true, true, false)
		case err := <-done:
			t.Fatalf("change skipped human decision: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
