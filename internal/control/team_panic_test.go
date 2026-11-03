package control

import (
	"context"
	"testing"
	"time"

	"deepzora/internal/event"
	"deepzora/internal/sessioninbox"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func TestTeamPanicSettlesMessageReplyAndGroupTurn(t *testing.T) {
	for _, kind := range []team.DeliveryKind{team.DeliverMessage, team.DeliverReply, team.DeliverGroupTurn} {
		t.Run(string(kind), func(t *testing.T) {
			c, svc, store, chief, ada := teamControllerFixture(t, &fakeTurnRunner{}, nil)
			x, _, err := store.OpenExchange(t.Context(), teamroster.Exchange{From: chief, To: ada, ReplyTo: teamroster.ReplyToUser}, "read")
			if err != nil {
				t.Fatal(err)
			}
			extra := map[string]string{team.ExtraKind: string(kind), team.ExtraExchange: x.ID, team.ExtraCeiling: "read-only"}
			childID, groupID := "", ""
			if kind == team.DeliverReply {
				child, _, err := store.OpenExchange(t.Context(), teamroster.Exchange{From: ada, To: chief, Parent: x.ID, ReplyTo: teamroster.ReplyToSender}, "child")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.SetExchangeState(t.Context(), child.ID, teamroster.ExchangeReplied, time.Now()); err != nil {
					t.Fatal(err)
				}
				childID = child.ID
				extra[team.ExtraExchange], extra[team.ExtraServing] = child.ID, x.ID
			}
			if kind == team.DeliverGroupTurn {
				_, err := store.Update(t.Context(), func(r *teamroster.Roster) error {
					groupID = r.AddGroup(teamroster.Group{Name: "discussion", Mode: teamroster.ModeEveryone, Members: []string{chief, ada}, CreatedBy: teamroster.ByUser}).ID
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				c.team.mu.Lock()
				c.team.seat = team.Seat{Service: svc, Ref: team.SeatRef{Agent: ada, Group: groupID}}
				c.team.mu.Unlock()
				if err := store.SaveRound(t.Context(), groupID, teamroster.RoundState{Round: 1, Step: 1, Speaking: &teamroster.Speaking{Slot: teamroster.Slot{Member: ada, Cause: teamroster.CauseUser}, Key: "step-1"}}); err != nil {
					t.Fatal(err)
				}
				extra[team.ExtraStep] = "step-1"
			}
			started := make(chan struct{})
			c.runGuarded(func(ctx context.Context) error {
				return c.runTeamInbox(ctx, sessioninbox.PromptEnvelope{Extra: extra}, func(context.Context) error { close(started); panic("runner failure") })
			})
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("not started")
			}
			waitIdle(t, c)
			if c.RuntimeStateSnapshot().TurnStatus != event.TurnFailed {
				t.Fatal("panic lost failed terminal outcome")
			}
			if c.effectiveApprovalMode() != ToolApprovalDangerFullAccess || c.currentTeamTurn().ceiling != "" {
				t.Fatal("panic leaked turn permission or identity")
			}
			if kind == team.DeliverGroupTurn {
				round, err := store.Round(groupID)
				if err != nil || round.Speaking != nil {
					t.Fatalf("group stuck: %+v %v", round, err)
				}
				lines, err := store.Lines(groupID, 0, 0)
				if err != nil || len(lines) != 1 || lines[0].Event != teamroster.EventFailed {
					t.Fatalf("group failure missing: %+v %v", lines, err)
				}
			} else {
				book, err := store.Exchanges()
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range book {
					if item.ID == x.ID && item.State != teamroster.ExchangeFailed {
						t.Fatalf("panic left parent pending: %+v", item)
					}
					if item.ID == childID && item.Returned {
						t.Fatal("panic pretended to consume reply")
					}
				}
			}
		})
	}
}
