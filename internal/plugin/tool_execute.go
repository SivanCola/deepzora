package plugin

import (
	"context"
	"deepzora/internal/secrets"
	"deepzora/internal/tool"
	"encoding/json"
	"fmt"
)

func (t *remoteTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	text, _, err := t.ExecuteWithImages(ctx, args)
	return text, err
}

// ExecuteWithImages implements tool.ImageTool: MCP results may carry image
// content items, which callers with a structural image channel (the agent)
// forward to vision models instead of relying on the text placeholders alone.
func (t *remoteTool) ExecuteWithImages(ctx context.Context, args json.RawMessage) (string, []string, error) {
	if secrets.ContainsPlaceholderJSON(args) {
		return "", nil, fmt.Errorf("masked secret placeholders cannot be sent to MCP tools; use an authorized credential mechanism")
	}
	res, err := t.callRaw(ctx, args)
	if err != nil {
		return "", nil, err
	}
	stampMCPAppResult(ctx, t, res)
	return parseToolResultWithSchema(res, t.outputSchema)
}

// ExecuteForApp returns the complete standard CallToolResult to an MCP App.
// The text and isError flag are separate host-local projections used for hooks
// and transcript events; an MCP isError result remains a successful bridge
// response so the App receives its structured fields and metadata.
func (t *remoteTool) ExecuteForApp(ctx context.Context, args json.RawMessage) (json.RawMessage, string, bool, error) {
	if secrets.ContainsPlaceholderJSON(args) {
		return nil, "", false, fmt.Errorf("masked secret placeholders cannot be sent to MCP tools")
	}
	res, err := t.callRaw(ctx, args)
	if err != nil {
		return nil, "", false, err
	}
	res, err = tool.ValidateMCPAppCallResult(res)
	if err != nil {
		return nil, "", false, err
	}
	var status struct {
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &status); err != nil {
		return nil, "", false, fmt.Errorf("decode MCP App tool result: %w", err)
	}
	text, _, parseErr := parseToolResultForApp(res)
	if parseErr != nil && !status.IsError {
		return nil, "", false, parseErr
	}
	return res, text, status.IsError, nil
}
