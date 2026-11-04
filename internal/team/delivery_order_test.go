package team

import (
	"context"
	"testing"

	"deepzora/internal/teamroster"
)

type immediatePane struct{ deliver func(Delivery) error }

func (p immediatePane) Deliver(d Delivery) error { return p.deliver(d) }
func (immediatePane) Cancel()                    {}

type immediatePanes struct{ pane Pane }

func (p immediatePanes) Ensure(context.Context, SeatRef) (Pane, error) { return p.pane, nil }
func (p immediatePanes) Live(SeatRef) (Pane, bool)                     { return p.pane, true }

func TestReceiverCanSettleBeforeDeliveryReturns(t *testing.T) {
	h := newHarness(t, "Chief", "Ada")
	h.svc.panes = immediatePanes{pane: immediatePane{deliver: func(d Delivery) error {
		return h.svc.Settled(t.Context(), SeatRef{Agent: h.id("Ada")}, Settlement{Serving: d.Serving, Outcome: OutcomeCompleted, FinalText: "Already finished"})
	}}}
	r, err := h.svc.MessageAgent(t.Context(), Caller{Agent: h.id("Chief")}, MessageRequest{To: h.id("Ada"), Message: "Quick request", ReplyTo: teamroster.ReplyToUser})
	if err != nil {
		t.Fatalf("receiver completion was overwritten by delivery: %v", err)
	}
	book, err := h.store.Exchanges()
	if err != nil || len(book) != 1 || book[0].ID != r.Exchange || book[0].State != teamroster.ExchangeReplied || !book[0].Returned {
		t.Fatalf("settlement lost: %+v %v", book, err)
	}
}
