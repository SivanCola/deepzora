package control

import (
	"context"
	"errors"
	"strings"
	"time"

	"deepzora/internal/agent"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/sandbox"
	"deepzora/internal/sessioninbox"
	"deepzora/internal/team"
	"deepzora/internal/tool"
)

type teamInputKey struct{}

// DeliverTeam acknowledges only the durable inbox commit. Dispatch runs off
// the delivery stack: group admission may still own the group-round lock.
func (c *Controller) DeliverTeam(d team.Delivery) error {
	if c.team.bound() == nil {
		return errTeamUnbound
	}
	extra := map[string]string{
		team.ExtraKind: string(d.Kind), team.ExtraFrom: d.From,
		team.ExtraExchange: d.Exchange, team.ExtraServing: strings.Join(d.Serving, ","),
		team.ExtraStep: d.Step, team.ExtraGroup: d.Group, team.ExtraCeiling: d.Ceiling,
	}
	_, err := c.EnqueueInbox(InboxRequest{Source: "team:" + d.From,
		Idempotency: d.Key, Display: d.Display, Raw: d.Text, Submit: d.Text, Extra: extra})
	if err == nil {
		c.mu.Lock()
		if !c.closed {
			c.autosaveWG.Go(c.NotifyInboxRuntimeReady)
		}
		c.mu.Unlock()
	}
	return err
}

func (c *Controller) runTeamInbox(ctx context.Context, env sessioninbox.PromptEnvelope, run func(context.Context) error) (err error) {
	kind := team.DeliveryKind(env.Extra[team.ExtraKind])
	if kind == "" {
		return run(ctx)
	}
	seat := c.team.bound()
	if seat == nil {
		return errTeamUnbound
	}
	svc, ref := seat.Team()
	if svc == nil {
		return errTeamUnbound
	}
	pending := true
	switch kind {
	case team.DeliverMessage:
		pending, err = svc.MessagePending(ref, env.Extra[team.ExtraExchange])
	case team.DeliverGroupTurn:
		pending, err = svc.GroupTurnPending(ref, env.Extra[team.ExtraStep])
	case team.DeliverReply:
	default:
		return tool.Refusal{Code: "team.invalid", Message: "Unknown team delivery kind."}
	}
	if err != nil {
		return err
	}
	if !pending {
		return tool.Refusal{Code: "team.interrupted", Message: "This queued team work has already settled."}
	}
	t := teamTurn{ceiling: env.Extra[team.ExtraCeiling]}
	switch kind {
	case team.DeliverMessage:
		t.serving = []string{env.Extra[team.ExtraExchange]}
	case team.DeliverGroupTurn:
		t.steps = []string{env.Extra[team.ExtraStep]}
	case team.DeliverReply:
		t.consumed = []string{env.Extra[team.ExtraExchange]}
		if parent := env.Extra[team.ExtraServing]; parent != "" {
			t.serving = strings.Split(parent, ",")
		}
	}
	c.team.mu.Lock()
	c.team.turn = t
	c.team.mu.Unlock()
	defer func() { c.team.mu.Lock(); c.team.turn = teamTurn{}; c.team.mu.Unlock() }()
	release := c.holdTeamCeiling(t.ceiling)
	defer release()
	ctx = context.WithValue(ctx, teamInputKey{}, true)
	if t.ceiling != "" {
		mode := c.effectiveApprovalMode()
		ctx = tool.WithExecutionCeiling(ctx, mode)
		ctx = sandbox.WithPermissionPreset(ctx, mode)
	}
	previousID, _ := teamFinalAnswer(c.History())
	err = run(ctx)
	outcome := team.OutcomeCompleted
	code := ""
	if ctx.Err() != nil {
		outcome, code = team.OutcomeCancelled, "turn.cancelled"
	} else if err != nil {
		outcome, code = team.OutcomeFailed, "turn.failed"
	}
	answer := ""
	if err == nil {
		if id, text := teamFinalAnswer(c.History()); id != previousID {
			answer = text
		}
	}
	settled := svc.Settled(context.Background(), ref, team.Settlement{
		Serving: t.serving, Consumed: t.consumed, Steps: t.steps,
		Outcome: outcome, Code: code, FinalText: answer,
	})
	return errors.Join(err, settled)
}

func teamFinalAnswer(messages []provider.Message) (string, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == provider.RoleUser {
			return "", ""
		}
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.ID, m.Content
		}
	}
	return "", ""
}

// Team turn identities follow Controller admission, not model-provided text.
func teamHostInput(ctx context.Context) context.Context {
	return agent.WithInputMessageOrigin(ctx, provider.MessageOriginHost)
}

func (c *Controller) observeTeamEvent(e event.Event) {
	seat := c.team.bound()
	if seat == nil {
		return
	}
	svc, ref := seat.Team()
	if svc == nil || ref.Group == "" {
		return
	}
	kind := team.ObserveEvent
	switch e.Kind {
	case event.ToolStarted:
		kind = team.ObserveToolStarted
	case event.ToolResult:
		kind = team.ObserveToolFinished
	}
	svc.Observed(ref, team.Observation{Kind: kind, At: time.Now()})
}
