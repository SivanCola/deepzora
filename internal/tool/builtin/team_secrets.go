package builtin

import (
	"context"
	"deepzora/internal/tool"
	"encoding/json"
)

func teamSecretTool(link *tool.TeamLink) tool.Tool {
	return &teamTool{link: link, name: "request_secret", readOnly: true, description: "Ask the user to save or authorize a named Team secret in your private Team conversation. Never request its value in chat. Use a stable request_id for retries. The value can only reach directly referenced command environment variables after authorization.", schema: `{"type":"object","properties":{"name":{"type":"string","pattern":"^[A-Z][A-Z0-9_]{0,39}$"},"purpose":{"type":"string","maxLength":200},"request_id":{"type":"string","minLength":8,"maxLength":128}},"required":["name","purpose","request_id"],"additionalProperties":false}`, run: func(ctx context.Context, dir tool.TeamDirectory, args json.RawMessage) (string, error) {
		var input struct {
			Name      string `json:"name"`
			Purpose   string `json:"purpose"`
			RequestID string `json:"request_id"`
		}
		if err := decodeTeamArgs("request_secret", args, &input); err != nil {
			return "", err
		}
		service, ok := dir.(tool.TeamSecretDirectory)
		if !ok {
			return "", tool.Refusal{Code: "team.secrets_unavailable", Message: "This host cannot handle Team secret requests."}
		}
		return service.RequestTeamSecret(ctx, input.Name, input.Purpose, input.RequestID)
	}}
}
