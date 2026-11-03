package builtin

import (
	"deepzora/internal/secrets"
	"deepzora/internal/tool"
	"encoding/json"
)

func rejectMaskedWrite(args json.RawMessage) error {
	if secrets.ContainsPlaceholderJSON(args) {
		return tool.Refusal{Code: "secret.placeholder", Message: "A masked secret placeholder is not a real credential. Use an authorized direct environment reference; do not write or execute the placeholder."}
	}
	return nil
}
