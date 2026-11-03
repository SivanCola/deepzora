package session

import (
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"encoding/json"
)

func maskSessionEvent(event Event) json.RawMessage {
	mask := secrets.Masker()
	if mask == nil {
		return event.Payload
	}
	projected := mask.JSONStrings(event.Payload)
	if event.Kind != "message/complete" && event.Kind != "message/upsert" && event.Kind != "message/replace" {
		return projected
	}
	var payload struct {
		Message provider.Message `json:"message"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil {
		return projected
	}
	messages := provider.MaskMessages([]provider.Message{payload.Message}, provider.SignedProjection{})
	if !messages[0].SecretReplayInvalidated {
		return projected
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(projected, &fields) != nil {
		return projected
	}
	fields["message"], _ = json.Marshal(messages[0])
	out, err := json.Marshal(fields)
	if err != nil {
		return projected
	}
	return out
}
