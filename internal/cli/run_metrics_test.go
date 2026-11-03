package cli

import (
	"testing"

	"deepzora/internal/event"
	"deepzora/internal/provider"
)

func TestMetricsSinkForwardsEachEventOnce(t *testing.T) {
	forwarded := 0
	s := &metricsSink{inner: event.FuncSink(func(event.Event) { forwarded++ })}
	s.Emit(event.Event{Kind: event.Text, Text: "x"})
	if forwarded != 1 {
		t.Fatalf("inner sink saw %d emissions for one event, want 1", forwarded)
	}
}

func TestMetricsSinkUsesProviderCacheWriteCost(t *testing.T) {
	s := &metricsSink{inner: event.Discard}
	s.Emit(event.Event{
		Kind: event.Usage,
		Usage: &provider.Usage{
			CacheMissTokens:        500_000,
			CacheWriteTokens:       100_000,
			CacheWriteBilledTokens: 200_000,
		},
		Pricing: &provider.Pricing{Input: 2},
	})
	if got := s.Snapshot().Cost; got != 1.2 {
		t.Fatalf("metrics cost = %f, want 1.2", got)
	}
}

func TestMetricsSinkAccumulatesSilentReasoningRecovery(t *testing.T) {
	s := &metricsSink{inner: event.Discard}
	s.RecordProtocolRecovery(event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningDetected})
	s.RecordProtocolRecovery(event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningRetryAttempted})
	s.RecordProtocolRecovery(event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningRetryRecovered})
	s.RecordProtocolRecovery(event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningRetryReplaced})
	s.RecordProtocolRecovery(event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningRetrySuppressed})
	s.RecordProtocolRecovery(event.ProtocolRecoveryAudit{Kind: event.ProtocolRecoveryMissingReasoningFallback})

	if s.m.MissingReasoningDetected != 1 || s.m.MissingReasoningRetries != 1 || s.m.MissingReasoningRecovered != 1 || s.m.MissingReasoningReplaced != 1 || s.m.MissingReasoningSuppressed != 1 || s.m.MissingReasoningFallbacks != 1 {
		t.Fatalf("reasoning recovery metrics = %+v", s.m)
	}
	if s.m.Steps != 1 {
		t.Fatalf("retry model-call step = %d, want 1", s.m.Steps)
	}
}

func TestMetricsSinkAccountsToolCallsAndRetries(t *testing.T) {
	s := &metricsSink{inner: event.Discard}

	s.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{Name: "read_file"}})
	s.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{Name: "read_file", DurationMs: 12}})
	s.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{Name: "read_file", DurationMs: 8}})
	s.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{Name: "bash", Err: "exit status 1", DurationMs: 30}})
	s.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{Name: "grep", ParentID: "task-1", DurationMs: 5}})
	s.Emit(event.Event{Kind: event.Retrying, RetryAttempt: 1, RetryMax: 3})

	if s.m.ToolCalls != 4 {
		t.Fatalf("tool calls = %d, want 4 (dispatch must not count)", s.m.ToolCalls)
	}
	if s.m.ToolFailures != 1 {
		t.Fatalf("tool failures = %d, want 1", s.m.ToolFailures)
	}
	if s.m.ToolDurationMs != 55 {
		t.Fatalf("tool duration = %dms, want 55", s.m.ToolDurationMs)
	}
	if s.m.SubagentToolCalls != 1 {
		t.Fatalf("subagent tool calls = %d, want 1", s.m.SubagentToolCalls)
	}
	if s.m.Retries != 1 {
		t.Fatalf("retries = %d, want 1", s.m.Retries)
	}
	if s.m.ToolCallsByName["read_file"] != 2 || s.m.ToolCallsByName["bash"] != 1 {
		t.Fatalf("calls by name = %v", s.m.ToolCallsByName)
	}
	if s.m.ToolFailuresByName["bash"] != 1 {
		t.Fatalf("failures by name = %v, want bash 1", s.m.ToolFailuresByName)
	}
	if _, ok := s.m.ToolFailuresByName["read_file"]; ok {
		t.Fatalf("a successful tool must not appear in failures: %v", s.m.ToolFailuresByName)
	}
}
