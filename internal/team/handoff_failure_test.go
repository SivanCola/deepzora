package team

import (
	"context"
	"errors"
	"testing"

	"deepzora/internal/teamroster"
)

func TestFailedReplyInterruptsAncestorsWithoutPretendingConsumption(t *testing.T) {
	for _, allUnavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "one seat", true: "all seats"}[allUnavailable], func(t *testing.T) {
			h := newHarness(t, "Chief", "Ada", "Ben", "Cy")
			root := h.send("Chief", "Ada", "root").Exchange
			parent := h.send("Ada", "Ben", "parent", root).Exchange
			child := h.send("Ben", "Cy", "child", parent).Exchange
			h.settle("Ada", Settlement{Serving: []string{root}, Outcome: OutcomeCompleted})
			h.settle("Ben", Settlement{Serving: []string{parent}, Outcome: OutcomeCompleted})
			h.panes.broke = map[SeatRef]bool{{Agent: h.id("Ben")}: true}
			if allUnavailable {
				h.panes.broke[SeatRef{Agent: h.id("Ada")}] = true
				h.panes.broke[SeatRef{Agent: h.id("Chief")}] = true
			}
			err := h.svc.Settled(t.Context(), SeatRef{Agent: h.id("Cy")}, Settlement{Serving: []string{child}, Outcome: OutcomeCompleted, FinalText: "unseen answer"})
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("delivery failure: %v", err)
			}
			for _, id := range []string{root, parent} {
				x := h.exchange(id)
				if x.State != teamroster.ExchangeInterrupted || x.Returned {
					t.Fatalf("ancestor falsely completed or consumed: %+v", x)
				}
			}
			if h.exchange(child).Returned {
				t.Fatal("failed reply counted as consumed")
			}
			h.panes.broke = nil
			h.settle("Ben", Settlement{Serving: []string{parent}, Outcome: OutcomeCompleted})
			if err := h.svc.Recover(t.Context()); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{root, parent} {
				if h.exchange(id).State != teamroster.ExchangeInterrupted {
					t.Fatal("late completion/restart resurrected ancestor")
				}
			}
		})
	}
}

func TestParallelServicesCannotExceedOpenLimit(t *testing.T) {
	h := newHarness(t, "Chief", "Ada", "Ben")
	for range MaxOpenPerSender - 1 {
		if _, _, err := h.store.OpenExchange(t.Context(), teamroster.Exchange{From: h.id("Chief"), To: h.id("Ada"), ReplyTo: teamroster.ReplyToSender}, "seed"); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, turn := range []string{"one", "two"} {
		svc := New(Options{Store: teamroster.Store{Dir: h.store.Dir}, Panes: h.panes})
		go func() {
			<-start
			_, err := svc.MessageAgent(context.Background(), Caller{Agent: h.id("Chief"), Turn: turn}, MessageRequest{To: h.id("Ben"), Message: turn, RequestID: turn})
			done <- err
		}()
	}
	close(start)
	a, b := <-done, <-done
	if !((a == nil && errors.Is(b, ErrOpenLimit)) || (b == nil && errors.Is(a, ErrOpenLimit))) {
		t.Fatalf("admission: %v / %v", a, b)
	}
}

func TestRecoverUndeliveredReplyReleasesQuotaOnlyAfterConsumption(t *testing.T) {
	h := newHarness(t, "Chief", "Ada")
	var first string
	for range MaxOpenPerSender {
		x, _, err := h.store.OpenExchange(t.Context(), teamroster.Exchange{From: h.id("Chief"), To: h.id("Ada"), ReplyTo: teamroster.ReplyToSender}, "seed")
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = x.ID
		}
		if _, err := h.store.SetExchangeState(t.Context(), x.ID, teamroster.ExchangeReplied, h.clock.now()); err != nil {
			t.Fatal(err)
		}
	}
	checkFull := func() {
		t.Helper()
		_, err := h.svc.MessageAgent(t.Context(), Caller{Agent: h.id("Chief")}, MessageRequest{To: h.id("Ada"), Message: "more"})
		if !errors.Is(err, ErrOpenLimit) {
			t.Fatalf("unconsumed answers bypassed limit: %v", err)
		}
	}
	checkFull()
	for range 2 {
		if err := h.svc.Recover(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.inbox("Chief"); len(got) != MaxOpenPerSender {
		t.Fatalf("duplicate recovery replies: %d", len(got))
	} else {
		contains(t, got[0].Text, `status="interrupted"`, `code="team.reply_unavailable"`)
	}
	if len(h.inbox("Ada")) != 0 {
		t.Fatal("recovery reran original work")
	}
	checkFull()
	h.settle("Chief", Settlement{Consumed: []string{first}, Outcome: OutcomeCompleted})
	h.send("Chief", "Ada", "quota available")
}

func TestRecoveryDoesNotReplaceAnAlreadyDeliveredAnswer(t *testing.T) {
	h := newHarness(t, "Chief", "Ada")
	x := h.send("Chief", "Ada", "answer")
	h.settle("Ada", Settlement{Serving: []string{x.Exchange}, Outcome: OutcomeCompleted, FinalText: "persisted answer"})
	before := h.inbox("Chief")[0]
	for range 2 {
		if err := h.svc.Recover(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	after := h.inbox("Chief")
	if len(after) != 1 || after[0].Text != before.Text || h.exchange(x.Exchange).Returned {
		t.Fatal("recovery replaced or consumed the existing answer")
	}
	if len(h.inbox("Ada")) != 1 {
		t.Fatal("recovery reran original work")
	}
}
