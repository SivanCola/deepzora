package secrets

import (
	"encoding/json"
	"strings"
)

func ContainsPlaceholderJSON(raw json.RawMessage) bool {
	if strings.Contains(string(raw), markOpen) {
		return true
	}
	if !json.Valid(raw) {
		return false
	}
	found := false
	jsonScalars(raw, func(_, _ int, text string) { found = found || strings.Contains(text, markOpen) })
	return found
}
