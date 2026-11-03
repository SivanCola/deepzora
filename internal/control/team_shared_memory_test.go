package control

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"deepzora/internal/event"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"deepzora/internal/tool"
)

func TestSharedMemoryAlwaysAsksFreshAndFencesRosterChanges(t *testing.T) {
	store := teamroster.Store{Dir: t.TempDir()}
	svc := team.New(team.Options{Store: store})
	roster, err := svc.Bootstrap(t.Context(), "Chief", "")
	if err != nil {
		t.Fatal(err)
	}
	chief := roster.Agents[0].ID
	requests := make(chan event.Approval, 4)
	c := newCurrentOwnedTestController(t, Options{Team: TeamOptions{Seat: team.Seat{Service: svc, Ref: team.SeatRef{Agent: chief}}}, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			requests <- e.Approval
		}
	})})
	c.EnableInteractiveApproval()
	c.SetToolApprovalMode(ToolApprovalDangerFullAccess)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for i := range 3 {
		done := make(chan error, 1)
		go func() {
			_, err := (teamDirectory{c}).RememberShared(ctx, tool.TeamSharedMemoryRequest{Tier: "team", Operation: "save", Name: []string{"style", "stale", "plan-changed"}[i], Description: "writing preference", Body: "Use plain language"})
			done <- err
		}()
		select {
		case approval := <-requests:
			if !approval.Fresh || approval.Tool != TeamMemoryApprovalTool || !strings.Contains(string(approval.RawInput), "rosterRevision") {
				t.Fatalf("memory did not bind a fresh decision: %+v", approval)
			}
			if i == 1 {
				if _, err := store.Update(ctx, func(r *teamroster.Roster) error { r.Agents[0].Role = "Changed after review"; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if i == 2 {
				c.SetPlanMode(true)
			}
			c.Approve(approval.ID, true, true, false)
		case err := <-done:
			t.Fatalf("shared write bypassed a human: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		err := <-done
		if i == 0 && err != nil {
			t.Fatal(err)
		}
		if i > 0 && err == nil {
			t.Fatal("stale memory approval survived a roster or Plan change")
		}
	}
	mem, err := svc.SharedMemory(team.Caller{Agent: chief}, team.TierTeam, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mem.ListAll()) != 1 {
		t.Fatal("a stale decision partially wrote memory")
	}
}

func TestSharedMemoryRefusesCredentialsAndReadOnlyDelegationBeforeAsking(t *testing.T) {
	c, _, _, _, _ := teamControllerFixture(t, &fakeTurnRunner{}, nil)
	for _, test := range []struct {
		ctx  context.Context
		body string
		code string
	}{
		{t.Context(), "api_key=sk-Abc1234567890abcdefghijklmnop", "team.credential"},
		{tool.WithExecutionCeiling(t.Context(), ToolApprovalReadOnly), "a preference", "team.memory_read_only"},
	} {
		_, err := (teamDirectory{c}).RememberShared(test.ctx, tool.TeamSharedMemoryRequest{Tier: "team", Operation: "save", Description: "preference", Body: test.body})
		var refusal tool.Refusal
		if !errors.As(err, &refusal) || refusal.Code != test.code {
			t.Fatalf("unexpected shared memory boundary: %v", err)
		}
	}
}
