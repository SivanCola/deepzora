package agent

import (
	"testing"

	"deepzora/internal/provider"
)

func TestSubagentStoreDuplicateTerminalOutcomeIsIdempotent(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir())
	spec := testSubagentSpec(t, "explore")
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	defer run.Release()
	answer := "same terminal answer"
	appendSubagentMessages(t, run, provider.Message{Role: provider.RoleAssistant, Content: answer})
	outcome := SubagentOutcome{Ref: run.Ref, Status: SubagentOutcomePartial, ErrorCode: "completion_uncertain", Retryable: true}
	if err := store.SaveOutcome(run, outcome); err != nil {
		t.Fatalf("first SaveOutcome: %v", err)
	}
	if err := store.SaveOutcome(run, outcome); err != nil {
		t.Fatalf("duplicate SaveOutcome: %v", err)
	}
	if err := store.SaveOutcome(run, SubagentOutcome{Ref: run.Ref, Status: SubagentOutcomeFailed, ErrorCode: "provider_error"}); err == nil {
		t.Fatal("different terminal outcome overwrote a persisted partial outcome")
	}
	meta, err := store.LoadMeta(run.Ref)
	if err != nil || meta.Outcome != string(SubagentOutcomePartial) || meta.Status != SubagentFailed || !meta.Retryable {
		t.Fatalf("duplicate terminal metadata = %+v/%v", meta, err)
	}
}
