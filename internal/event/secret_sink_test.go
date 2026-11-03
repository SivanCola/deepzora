package event

import (
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"strings"
	"testing"
)

func TestSecretSinkMasksCrossEventValuesAndPreservesTypedUsage(t *testing.T) {
	secrets.WatchValues(t.Name(), func() []secrets.Labeled {
		return []secrets.Labeled{{Label: "STREAM_EVENT", Value: "event-private-value-7319"}, {Label: "PIN_EVENT", Value: "7319"}}
	})
	events := []Event{}
	sink := NewSecretSink(FuncSink(func(e Event) { events = append(events, e) }))
	sink.Emit(Event{Kind: Text, MessageID: "secret-message", Text: "before event-pri"})
	sink.Emit(Event{Kind: Usage, Usage: &provider.Usage{TotalTokens: 7319, RequestCount: 1}})
	sink.Emit(Event{Kind: Text, MessageID: "secret-message", Text: "vate-value-7319 after"})
	sink.Emit(Event{Kind: Usage, Usage: &provider.Usage{TotalTokens: 7319, RequestCount: 1}})
	sink.Emit(Event{Kind: TurnDone})
	var text strings.Builder
	for _, e := range events {
		if e.Kind == Text {
			text.WriteString(e.Text)
		}
		if e.Kind == Usage && e.Usage.TotalTokens != 7319 {
			t.Fatal("masking corrupted a typed usage counter")
		}
	}
	if got := text.String(); got != "before [secret:STREAM_EVENT] after" {
		t.Fatalf("cross-event value leaked: %q", got)
	}
}

var _ OptionalSinkCapabilities = (*SecretSink)(nil)
var _ CheckedSink = (*SecretSink)(nil)
var _ OptionalSinkCapabilities = (*TeamUsageSink)(nil)

func TestSecretSinkCommittedProofKeepsInvalidationMarker(t *testing.T) {
	value := "committed-proof-private-8190"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "COMMITTED", Value: value}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	var received Event
	sink := NewSecretSink(FuncSink(func(e Event) { received = e }))
	sink.Emit(Event{Kind: ToolResult, CommittedMessage: &provider.Message{Role: provider.RoleAssistant, ReasoningContent: value, ReasoningSignature: "proof"}})
	if received.CommittedMessage == nil || !received.CommittedMessage.SecretReplayInvalidated || strings.Contains(received.CommittedMessage.ReasoningContent, value) {
		t.Fatal("committed message lost its proof invalidation marker")
	}
}

func TestSecretSinkFlushPreservesShortValueWordBoundary(t *testing.T) {
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "SHORT_FLUSH", Value: "4496"}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	var text strings.Builder
	sink := NewSecretSink(FuncSink(func(e Event) {
		if e.Kind == Text {
			text.WriteString(e.Text)
		}
	}))
	sink.Emit(Event{Kind: Text, Text: "order-x"})
	sink.Emit(Event{Kind: Text, Text: "4496"})
	sink.Emit(Event{Kind: TurnDone})
	if text.String() != "order-x4496" {
		t.Fatalf("standalone projection changed ordinary order text: %q", text.String())
	}
}
