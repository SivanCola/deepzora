package session

import (
	"bytes"
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"encoding/json"
	"strings"
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
	if payload.Message.SecretReplayInvalidated || !messages[0].SecretReplayInvalidated {
		return projected
	}
	return markSecretReplayPayload(projected)
}

// Only insert the host marker. Keep every unrelated byte, unknown member and
// duplicate key rather than decoding/re-encoding the persisted envelope.
func markSecretReplayPayload(raw json.RawMessage) json.RawMessage {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return raw
	}
	var out bytes.Buffer
	last := 0
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return raw
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return raw
		}
		name, ok := key.(string)
		if !ok || !strings.EqualFold(name, "message") || len(value) < 2 || value[0] != '{' || value[len(value)-1] != '}' {
			continue
		}
		end := int(decoder.InputOffset())
		start := end - len(value)
		out.Write(raw[last:start])
		out.Write(value[:len(value)-1])
		if len(bytes.TrimSpace(value[1:len(value)-1])) > 0 {
			out.WriteByte(',')
		}
		out.WriteString(`"secret_replay_invalidated":true}`)
		last = end
	}
	if last == 0 {
		return raw
	}
	out.Write(raw[last:])
	return out.Bytes()
}
