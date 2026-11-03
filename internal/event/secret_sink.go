package event

import (
	"bytes"
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"sync"
)

type secretDeltaKey struct {
	kind                           Kind
	source, message, attempt, tool string
}
type secretDelta struct {
	stream   *secrets.Stream
	buffer   bytes.Buffer
	template Event
}
type SecretSink struct {
	AuditForwarder
	mu     sync.Mutex
	deltas map[secretDeltaKey]*secretDelta
}

func NewSecretSink(inner Sink) *SecretSink {
	return &SecretSink{AuditForwarder: AuditForwarder{Inner: inner}, deltas: map[secretDeltaKey]*secretDelta{}}
}
func (s *SecretSink) Emit(e Event) { _ = s.EmitChecked(e) }
func (s *SecretSink) EmitChecked(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mask := secrets.Masker()
	if mask == nil && len(s.deltas) == 0 {
		return EmitChecked(s.Inner, e)
	}
	key := secretDeltaKey{kind: e.Kind, source: e.Source, message: e.MessageID, attempt: e.AttemptID, tool: e.Tool.ID}
	if e.Kind == Text || e.Kind == Reasoning || e.Kind == ToolProgress {
		delta := s.deltas[key]
		if delta == nil {
			delta = &secretDelta{template: e}
			delta.stream = secrets.NewStream(&delta.buffer)
			s.deltas[key] = delta
		}
		delta.template = e
		text := e.Text
		if e.Kind == ToolProgress {
			text = e.Tool.Output
		}
		if _, err := delta.stream.Write([]byte(text)); err != nil {
			return err
		}
		if delta.buffer.Len() == 0 {
			return nil
		}
		output := delta.buffer.String()
		delta.buffer.Reset()
		if e.Kind == ToolProgress {
			e.Tool.Output = output
		} else {
			e.Text = output
		}
	} else {
		for key, delta := range s.deltas {
			if !secretDeltaFinished(key, e) {
				continue
			}
			if err := delta.stream.Flush(); err != nil {
				return err
			}
			if delta.buffer.Len() > 0 {
				tail := delta.template
				if tail.Kind == ToolProgress {
					tail.Tool.Output = delta.buffer.String()
				} else {
					tail.Text = delta.buffer.String()
				}
				if err := EmitChecked(s.Inner, maskStreamEvent(tail)); err != nil {
					return err
				}
			}
			delete(s.deltas, key)
		}
	}
	return EmitChecked(s.Inner, maskStreamEvent(e))
}
func maskStreamEvent(e Event) Event {
	projected := maskEvent(e)
	// These fields have already been projected by the stream, with its
	// original cross-block word boundaries. Re-masking the isolated delta
	// would turn an ordinary short-token suffix into a false secret hit.
	switch e.Kind {
	case Text, Reasoning:
		projected.Text = e.Text
	case ToolProgress:
		projected.Tool.Output = e.Tool.Output
	}
	return projected
}
func secretDeltaFinished(key secretDeltaKey, e Event) bool {
	if key.source != e.Source {
		return false
	}
	if e.Kind == TurnDone {
		return true
	}
	if e.Kind == ToolResult {
		return key.kind == ToolProgress && key.tool == e.Tool.ID
	}
	return e.Kind == Message && key.kind != ToolProgress && (e.MessageID == "" || key.message == e.MessageID)
}
func maskEvent(e Event) Event {
	projected := secrets.Project(e)
	if e.CommittedMessage != nil {
		messages := provider.MaskMessages([]provider.Message{*e.CommittedMessage}, provider.SignedProjection{})
		projected.CommittedMessage = &messages[0]
	}
	return projected
}
