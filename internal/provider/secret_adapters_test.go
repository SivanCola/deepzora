package provider_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"deepzora/internal/provider"
	"deepzora/internal/provider/anthropic"
	"deepzora/internal/provider/openai"
	"deepzora/internal/provider/responses"
	"deepzora/internal/secrets"
)

func TestNativeAdaptersRejectSecretInvalidatedProofBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	value := "adapter-signed-private-value"
	secrets.WatchValues(t.Name(), func() []secrets.Labeled { return []secrets.Labeled{{Label: "ADAPTER", Value: value}} })
	t.Cleanup(func() { secrets.UnwatchValues(t.Name()) })
	chat, err := openai.New(provider.Config{Name: "fixture", Model: "model", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := anthropic.New(provider.Config{Name: "fixture", Model: "model", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	response := responses.New(responses.Config{Name: "fixture", Model: "model", BaseURL: server.URL})
	for _, message := range []provider.Message{
		{Role: provider.RoleAssistant, ReasoningContent: value, ReasoningSignature: "proof"},
		{Role: provider.RoleAssistant, ThinkingBlocks: []provider.ThinkingBlock{{Type: "thinking", Thinking: value, Signature: "proof"}}},
		{Role: provider.RoleAssistant, ResponsesItems: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"` + value + `"}`)}},
	} {
		projected := provider.MaskMessages([]provider.Message{message}, provider.SignedProjection{})
		if !projected[0].SecretReplayInvalidated {
			t.Fatal("changed proof was not marked")
		}
		for _, adapter := range []provider.Provider{chat, claude, response} {
			if _, err := adapter.Stream(t.Context(), provider.Request{Messages: projected}); !errors.Is(err, provider.ErrSecretSignedReplay) {
				t.Fatalf("%s sent invalid proof: %v", adapter.Name(), err)
			}
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid proof reached HTTP")
	}
}
