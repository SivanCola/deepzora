package provider

import (
	"deepzora/internal/secrets"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestOutboundSecretProjectionCoversSchemasDuplicatesAndOriginalAdapterProofs(t *testing.T) {
	value := "outbound-private-value-9721"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "WIRE", Value: value}} })
	req := Request{Messages: []Message{{Role: RoleAssistant, Content: value, RawContent: value, ReasoningContent: value, ReasoningSignature: "proof", ThinkingBlocks: []ThinkingBlock{{Type: "thinking", Thinking: value, Signature: "proof"}}, ToolCalls: []ToolCall{{ID: "call", Name: "tool", Arguments: `{"key":"` + value + `","key":"clean"}`, ThoughtSignature: "thought"}}}}, Tools: []ToolSchema{{Name: "tool", Description: value, Parameters: json.RawMessage(`{"description":"` + value + `","description":"clean","n":12345678901234567890}`)}}}
	original, _ := json.Marshal(req)
	plain := MaskOutbound(req, SignedProjection{})
	encoded, _ := json.Marshal(plain)
	if strings.Contains(string(encoded), value) || !strings.Contains(string(encoded), `"n":12345678901234567890`) {
		t.Fatalf("outbound projection leaked or rewrote clean numeric bytes: %s", encoded)
	}
	if strings.Count(string(plain.Tools[0].Parameters), `"description"`) != 2 {
		t.Fatal("schema projection discarded a duplicate key")
	}
	signed := MaskOutbound(req, SignedProjection{Reasoning: true, ToolCalls: true})
	if signed.Messages[0].ReasoningContent != value || signed.Messages[0].ThinkingBlocks[0].Thinking != value || signed.Messages[0].ToolCalls[0].Arguments != req.Messages[0].ToolCalls[0].Arguments {
		t.Fatal("original adapter corrupted proof-bound bytes")
	}
	if signed.Messages[0].Content == value {
		t.Fatal("proof exception escaped into ordinary message content")
	}
	after, _ := json.Marshal(req)
	if !reflect.DeepEqual(original, after) {
		t.Fatal("projection mutated canonical history")
	}
	clean := Request{Messages: []Message{{Role: RoleUser, Content: "ordinary"}}, Tools: []ToolSchema{{Name: "tool", Parameters: json.RawMessage(`{ "type": "object", "default": 42 }`)}}}
	before, _ := json.Marshal(clean)
	after, _ = json.Marshal(MaskOutbound(clean, SignedProjection{}))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("clean provider request bytes changed")
	}
}
