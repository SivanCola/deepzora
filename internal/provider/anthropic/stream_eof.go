package anthropic

import (
	"fmt"
	"io"
	"time"

	"deepzora/internal/provider"
)

// streamScanEndError classifies why the SSE scanner stopped. A clean close
// after message_delta.stop_reason is a complete terminal: compatible gateways
// often omit message_stop, the same way OpenAI-compatible gateways omit [DONE]
// after finish_reason. A clean close with no stop_reason stays uncommitted.
func streamScanEndError(name string, stall provider.StreamStall, window time.Duration, scanErr error, stopReason string) error {
	switch stall {
	case provider.StreamStallIdle:
		err := fmt.Errorf("%s: stream stalled — no data for %s, connection likely dropped", name, window)
		return provider.StreamInterrupt(err, provider.StreamInterruptIdleTimeout)
	case provider.StreamStallNoOutput:
		err := fmt.Errorf("%s: stream stalled — connection alive but no model output for %s", name, window)
		return provider.StreamInterrupt(err, provider.StreamInterruptIdleTimeout)
	}
	if scanErr != nil {
		wrapped := fmt.Errorf("%s: read stream: %w", name, scanErr)
		if provider.IsConnReset(scanErr) {
			return provider.StreamInterrupt(wrapped, provider.ClassifyStreamInterrupt(scanErr))
		}
		return wrapped
	}
	if stopReason != "" {
		return nil
	}
	return provider.StreamInterrupt(
		fmt.Errorf("%s: stream ended before message_stop: %w", name, io.ErrUnexpectedEOF),
		provider.StreamInterruptPrematureEOF,
	)
}
