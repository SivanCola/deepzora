package session

import (
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPersistedSignedMessageMarksInvalidatedProofRatherThanReplayingChangedBytes(t *testing.T) {
	value := "proof-private-value-29184756"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "PROOF", Value: value}} })
	raw, _ := json.Marshal(map[string]any{"message": provider.Message{Role: provider.RoleAssistant, ThinkingBlocks: []provider.ThinkingBlock{{Type: "thinking", Thinking: value, Signature: "opaque-proof"}}, ReasoningState: provider.ReasoningComplete}})
	projected := maskSessionEvent(Event{Kind: "message/complete", Payload: raw})
	if strings.Contains(string(projected), value) {
		t.Fatal("signed content leaked into persistence")
	}
	var payload struct {
		Message provider.Message `json:"message"`
	}
	if err := json.Unmarshal(projected, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Message.SecretReplayInvalidated {
		t.Fatal("changed proof remained replayable")
	}
	if err := provider.CheckSecretSignedReplay(provider.Request{Messages: []provider.Message{payload.Message}}); !errors.Is(err, provider.ErrSecretSignedReplay) {
		t.Fatalf("invalid proof was not refused: %v", err)
	}
}

func TestPersistedUnsignedNumericArgumentsRemainValidJSONAfterMasking(t *testing.T) {
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "ARGUMENT_PIN", Value: "9546"}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	for _, kind := range []string{"message/complete", "message/upsert"} {
		message := provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call", Name: "fixture", Arguments: `{"pin":9546,"pin":"safe"}`}}}
		raw, _ := json.Marshal(map[string]any{"message": message})
		projected := maskSessionEvent(Event{Kind: kind, Payload: raw})
		var payload struct {
			Message provider.Message `json:"message"`
		}
		if err := json.Unmarshal(projected, &payload); err != nil {
			t.Fatal(err)
		}
		args := payload.Message.ToolCalls[0].Arguments
		if !json.Valid([]byte(args)) || args != `{"pin":"[secret:ARGUMENT_PIN]","pin":"safe"}` {
			t.Fatalf("%s corrupted nested arguments: %s", kind, args)
		}
	}
}

func TestProofMarkerPreservesEnvelopeBytesUnknownMembersAndDuplicateKeys(t *testing.T) {
	raw := json.RawMessage(`{ "message": {"future":1,"future":2,"content":"clean"}, "opaque":1, "opaque":2 }`)
	want := `{ "message": {"future":1,"future":2,"content":"clean","secret_replay_invalidated":true}, "opaque":1, "opaque":2 }`
	if got := string(markSecretReplayPayload(raw)); got != want {
		t.Fatalf("proof marker rewrote envelope: %s", got)
	}
}
