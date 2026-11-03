package secrets

import (
	"encoding/json"
	"slices"
	"strings"
)

// JSON masks every scalar, including keys and shadowed duplicate-key values.
// Unchanged tokens retain their bytes so clean schemas keep their cache prefix.
func (m *Mask) JSON(raw json.RawMessage) json.RawMessage {
	return m.projectJSON(raw, false)
}

// JSONStrings projects typed host envelopes while retaining structural member
// names and counters. Model-authored JSON documents (including nested argument
// strings) use JSON, which also projects their keys and numeric values.
func (m *Mask) JSONStrings(raw json.RawMessage) json.RawMessage { return m.projectJSON(raw, true) }
func (m *Mask) projectJSON(raw json.RawMessage, stringsOnly bool) json.RawMessage {
	if m == nil || len(raw) == 0 {
		return raw
	}
	if !json.Valid(raw) {
		return json.RawMessage(m.String(string(raw)))
	}
	var out []byte
	last := 0
	jsonScalars(raw, func(start, end int, value string) {
		if stringsOnly && (raw[start] != '"' || jsonMemberKey(raw, end)) {
			return
		}
		masked := m.String(value)
		// Tool arguments and schemas can be JSON documents carried inside a
		// typed string field. Project their scalar tokens before quoting the
		// outer string, so a numeric secret cannot leave invalid inner JSON.
		inner := strings.TrimSpace(value)
		if stringsOnly && raw[start] == '"' && (strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "[")) && json.Valid([]byte(value)) {
			masked = string(m.JSON(json.RawMessage(value)))
		}
		if masked == value {
			return
		}
		encoded, _ := json.Marshal(masked)
		out = append(out, raw[last:start]...)
		out = append(out, encoded...)
		last = end
	})
	if out == nil {
		return raw
	}
	return append(out, raw[last:]...)
}

func jsonMemberKey(raw []byte, end int) bool {
	for end < len(raw) && (raw[end] == ' ' || raw[end] == '\t' || raw[end] == '\n' || raw[end] == '\r') {
		end++
	}
	return end < len(raw) && raw[end] == ':'
}

// MarksJSON checks decoded arguments without dropping duplicate object keys.
func (m *Mask) MarksJSON(raw json.RawMessage) []string {
	if m == nil {
		return nil
	}
	if !json.Valid(raw) {
		return m.Marks(string(raw))
	}
	var out []string
	jsonScalars(raw, func(_, _ int, value string) {
		out = append(out, m.Marks(value)...)
	})
	slices.Sort(out)
	return slices.Compact(out)
}

// The caller validates the document; this walk preserves all tokens, including
// duplicate keys that decoding into a map would discard before masking.
func jsonScalars(raw []byte, visit func(int, int, string)) {
	for i := 0; i < len(raw); {
		start := i
		switch c := raw[i]; {
		case c == '"':
			i++
			for raw[i] != '"' {
				if raw[i] == '\\' {
					i++
				}
				i++
			}
			i++
			var value string
			_ = json.Unmarshal(raw[start:i], &value)
			visit(start, i, value)
		case c == '-' || c >= '0' && c <= '9':
			i++
			for i < len(raw) && numberByte(raw[i]) {
				i++
			}
			visit(start, i, string(raw[start:i]))
		default:
			i++
		}
	}
}

func numberByte(c byte) bool {
	return c >= '0' && c <= '9' || c == '.' || c == '-' || c == '+' || c == 'e' || c == 'E'
}
