package provider

import (
	"bytes"
	"deepzora/internal/secrets"
	"encoding/json"
	"errors"
	"slices"
)

// SignedProjection is supplied only by the original protocol adapter. Generic
// extension and broker receivers are never entitled to opaque signed bytes.
type SignedProjection struct {
	Reasoning, ToolCalls bool
	Items                []string
}

var ErrSecretSignedReplay = errors.New("secret masking changed signed replay content; start a new conversation instead of replaying its invalid proof")

func CheckSecretSignedReplay(req Request) error {
	_ = secrets.Masker()
	if err := secrets.BoundaryError(); err != nil {
		return err
	}
	for _, message := range req.Messages {
		if message.SecretReplayInvalidated {
			return ErrSecretSignedReplay
		}
	}
	return nil
}

func MaskOutbound(req Request, keep SignedProjection) Request {
	m := secrets.Masker()
	if m == nil {
		return req
	}
	req.Messages = maskOutboundMessages(m, req.Messages, keep)
	if len(req.Tools) > 0 {
		req.Tools = slices.Clone(req.Tools)
		for i := range req.Tools {
			req.Tools[i].Description = m.String(req.Tools[i].Description)
			req.Tools[i].Parameters = m.JSON(req.Tools[i].Parameters)
		}
	}
	return req
}
func MaskMessages(messages []Message, keep SignedProjection) []Message {
	return maskOutboundMessages(secrets.Masker(), messages, keep)
}
func maskOutboundMessages(m *secrets.Mask, messages []Message, keep SignedProjection) []Message {
	if m == nil {
		return messages
	}
	out := slices.Clone(messages)
	for i := range out {
		msg := &out[i]
		msg.Content = m.String(msg.Content)
		msg.RawContent = m.String(msg.RawContent)
		msg.Original = m.String(msg.Original)
		if !keep.Reasoning || msg.ReasoningSignature == "" {
			masked := m.String(msg.ReasoningContent)
			if msg.ReasoningSignature != "" && masked != msg.ReasoningContent {
				msg.SecretReplayInvalidated = true
			}
			msg.ReasoningContent = masked
		}
		if len(msg.ThinkingBlocks) > 0 {
			msg.ThinkingBlocks = slices.Clone(msg.ThinkingBlocks)
			for j := range msg.ThinkingBlocks {
				b := &msg.ThinkingBlocks[j]
				if !keep.Reasoning || b.Signature == "" {
					masked := m.String(b.Thinking)
					if b.Signature != "" && masked != b.Thinking {
						msg.SecretReplayInvalidated = true
					}
					b.Thinking = masked
					b.Data = m.String(b.Data)
					b.Signature = m.String(b.Signature)
				}
			}
		}
		if len(msg.ToolCalls) > 0 {
			msg.ToolCalls = slices.Clone(msg.ToolCalls)
			for j := range msg.ToolCalls {
				call := &msg.ToolCalls[j]
				if !keep.ToolCalls || call.ThoughtSignature == "" {
					masked := string(m.JSON(json.RawMessage(call.Arguments)))
					if call.ThoughtSignature != "" && masked != call.Arguments {
						msg.SecretReplayInvalidated = true
					}
					call.Arguments = masked
					call.ThoughtSignature = m.String(call.ThoughtSignature)
				}
			}
		}
		if len(msg.ResponsesItems) > 0 {
			msg.ResponsesItems = slices.Clone(msg.ResponsesItems)
			for j, item := range msg.ResponsesItems {
				var head struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(item, &head)
				if !slices.Contains(keep.Items, head.Type) {
					msg.ResponsesItems[j] = m.JSON(item)
					if head.Type == "reasoning" && !bytes.Equal(msg.ResponsesItems[j], item) {
						msg.SecretReplayInvalidated = true
					}
				}
			}
		}
		if len(msg.ServerSearch) > 0 {
			raw, _ := json.Marshal(msg.ServerSearch)
			var projected []ServerSearchCall
			if json.Unmarshal(m.JSON(raw), &projected) == nil {
				msg.ServerSearch = projected
			}
		}
	}
	return out
}
