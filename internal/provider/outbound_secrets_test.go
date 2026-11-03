package provider

import (
	"deepzora/internal/attachment"
	"deepzora/internal/secrets"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestImageLocatorsMaskSecretsWithoutMutatingHistoryOrBinaryPayload(t *testing.T) {
	value := "image-private-credential"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled {
		return []secrets.Labeled{{Label: "IMAGE", Value: value}, {Label: "PIN", Value: "AQID"}}
	})
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	inline := "data:image/png;base64,AQIDBA=="
	url := "https://example.test/image.png?token=" + value
	req := Request{Messages: []Message{
		{Role: RoleUser, Images: []string{url, "file-api-" + value, inline}},
		{Role: RoleUser, ImageInputs: []attachment.ImageInput{
			{Kind: attachment.KindURL, URL: url},
			{Kind: attachment.KindFiles, FilesID: "file-api-" + value},
			{Kind: attachment.KindAttachment, Attachment: &attachment.AttachmentRef{DisplayName: value}},
		}},
	}}
	before, _ := json.Marshal(req)
	masked := MaskOutbound(req, SignedProjection{})
	raw, _ := json.Marshal(masked)
	if strings.Contains(string(raw), value) {
		t.Fatal("image locator or metadata leaked")
	}
	if masked.Messages[0].Images[2] != inline {
		t.Fatal("masking corrupted inline image bytes")
	}
	after, _ := json.Marshal(req)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("masking mutated canonical image history")
	}
	clean := Request{Messages: []Message{{Role: RoleUser, Images: []string{inline, "https://example.test/clean.png", "file-api-clean"}}}}
	before, _ = json.Marshal(clean)
	after, _ = json.Marshal(MaskOutbound(clean, SignedProjection{}))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("clean image request bytes changed")
	}
}

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

func TestProofMetadataAndOpaqueBlocksAreOnlyPreservedByTheirOriginalAdapter(t *testing.T) {
	value := "opaque-proof-private-value"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "OPAQUE_PROOF", Value: value}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	for _, message := range []Message{
		{ReasoningContent: "clean", ReasoningSignature: value},
		{ThinkingBlocks: []ThinkingBlock{{Type: "thinking", Thinking: "clean", Signature: value}}},
		{ThinkingBlocks: []ThinkingBlock{{Type: "redacted_thinking", Data: value}}},
		{ToolCalls: []ToolCall{{ID: "call", Name: "fixture", Arguments: `{}`, ThoughtSignature: value}}},
	} {
		projected := MaskMessages([]Message{message}, SignedProjection{})[0]
		raw, _ := json.Marshal(projected)
		if strings.Contains(string(raw), value) || !projected.SecretReplayInvalidated {
			t.Fatalf("proof metadata leaked or remained replayable: %s", raw)
		}
		original := MaskMessages([]Message{message}, SignedProjection{Reasoning: true, ToolCalls: true})[0]
		if !reflect.DeepEqual(original, message) {
			t.Fatal("original adapter changed required proof bytes")
		}
	}
}

func TestMalformedSearchRawContentCannotBypassTextProjection(t *testing.T) {
	value := "search-raw-private-value"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "SEARCH_RAW", Value: value}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	message := Message{ServerSearch: []ServerSearchCall{{Query: value, Raw: json.RawMessage("invalid " + value)}}}
	projected := MaskMessages([]Message{message}, SignedProjection{})[0]
	if strings.Contains(projected.ServerSearch[0].Query, value) || strings.Contains(string(projected.ServerSearch[0].Raw), value) {
		t.Fatal("malformed raw search content bypassed masking")
	}
}
