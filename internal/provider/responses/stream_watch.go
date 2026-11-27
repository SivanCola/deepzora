package responses

import (
	"strings"

	"deepzora/internal/provider"
)

// watchResponsesEvent tells the stall watchdog what an event proves. Reasoning
// items are often hidden or summarized in sparse bursts, and a model may reason
// again after a text preamble, so anything reasoning-shaped re-arms the long
// window; text and tool-call deltas are output. Lifecycle events prove nothing.
func watchResponsesEvent(watch *provider.StreamWatchdog, event sseEvent) {
	switch {
	case strings.HasPrefix(event.Type, "response.reasoning"),
		event.Type == "response.output_item.added" && event.Item != nil && event.Item.Type == "reasoning":
		watch.Thinking()
	case event.Type == "response.output_text.delta",
		event.Type == "response.function_call_arguments.delta":
		watch.Output()
	}
}
