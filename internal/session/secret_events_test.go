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
