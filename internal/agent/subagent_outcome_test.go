package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"deepzora/internal/provider"
)

type closeFailureSubagentTranscript struct {
	SubagentTranscript
	err   error
	calls int
}

func (s *closeFailureSubagentTranscript) Close(ctx context.Context) error {
	s.calls++
	if s.SubagentTranscript != nil {
		return errors.Join(s.err, s.SubagentTranscript.Close(ctx))
	}
	return s.err
}

func TestSubagentCloseFailureCannotPublishCompletedOutcome(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir())
	run, err := store.PrepareFresh(testSubagentSpec(t, "review"))
	if err != nil {
		t.Fatal(err)
	}
	closeErr := errors.New("final archive close failed")
	run.Persistence = &closeFailureSubagentTranscript{SubagentTranscript: run.Persistence, err: closeErr}
	appendSubagentMessages(t, run, provider.Message{Role: provider.RoleAssistant, Content: "retained findings"})
	runner := &SubagentRunner{transcripts: store}
	output, err := runner.finishSubagentRun(run, "retained findings", nil)
	if !errors.Is(err, closeErr) || !strings.Contains(output, "status=failed") || !strings.Contains(output, "retained findings") {
		t.Fatalf("close failure misreported: %q %v", output, err)
	}
	meta, err := store.LoadMeta(run.Ref)
	if err != nil || meta.Status != SubagentFailed || meta.Outcome != string(SubagentOutcomeFailed) {
		t.Fatalf("close failure metadata: %+v %v", meta, err)
	}
}

func TestFailedSubagentSetupReleasesOwnerAndPreservesCloseError(t *testing.T) {
	cause, closeErr := errors.New("setup failed"), errors.New("close failed")
	persistence := &closeFailureSubagentTranscript{err: closeErr}
	released := 0
	run := &SubagentRun{Session: NewSession("system"), Persistence: persistence, release: func() { released++ }}
	_, err := (*SubagentRunner)(nil).finishSubagentRun(run, "", cause)
	if !errors.Is(err, cause) || !errors.Is(err, closeErr) || released != 1 || persistence.calls != 1 {
		t.Fatalf("release lost errors or ownership: %v, releases=%d closes=%d", err, released, persistence.calls)
	}
	if err := run.Release(); !errors.Is(err, closeErr) || released != 1 || persistence.calls != 1 {
		t.Fatalf("release retry was not idempotent: %v, releases=%d closes=%d", err, released, persistence.calls)
	}
}

func TestSubagentRunErrorPreservesPartialAnswerAndReference(t *testing.T) {
	run := &SubagentRun{Ref: "sa_partial", Session: NewSession("sys")}
	run.Session.Add(provider.Message{Role: provider.RoleAssistant, Content: "partial findings"})
	base := &IncompleteReadError{Reason: "child read is incomplete"}
	err := NewSubagentRunError(run, base)
	if !errors.Is(err, base) {
		t.Fatal("subagent error did not preserve its cause")
	}
	if err.Outcome.Status != SubagentOutcomePartial || !err.Outcome.Retryable {
		t.Fatalf("outcome = %+v, want retryable partial", err.Outcome)
	}
	out := err.SubagentOutput()
	for _, want := range []string{"status=partial", "retryable=true", "error_code=incomplete_read", "sa_partial", "partial findings"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q missing %q", out, want)
		}
	}
}

func TestSubagentRunErrorClassifiesCancellationAndProviderRetry(t *testing.T) {
	if code, status, retry := subagentErrorDisposition(context.Canceled); code != "cancelled" || status != SubagentOutcomeCancelled || retry {
		t.Fatalf("cancellation disposition = %s/%s/%v", code, status, retry)
	}
	if code, status, retry := subagentErrorDisposition(context.DeadlineExceeded); code != "cancelled" || status != SubagentOutcomeCancelled || retry {
		t.Fatalf("deadline disposition = %s/%s/%v", code, status, retry)
	}
	if code, status, retry := subagentErrorDisposition(&provider.APIError{Status: 503}); code != "provider_http_503" || status != SubagentOutcomeFailed || !retry {
		t.Fatalf("retryable provider disposition = %s/%s/%v", code, status, retry)
	}
	if _, _, retry := subagentErrorDisposition(&provider.APIError{Status: 401}); retry {
		t.Fatal("authentication failure must not be marked retryable")
	}
}

func TestParseSubagentOutcomeFromFormattedResult(t *testing.T) {
	outcome, ok := ParseSubagentOutcome("Subagent reference: sa_child\nSubagent outcome: status=partial retryable=true error_code=incomplete_read\n\nFinal answer:\nfindings")
	if !ok || outcome.Ref != "sa_child" || outcome.Status != SubagentOutcomePartial || !outcome.Retryable || outcome.ErrorCode != "incomplete_read" {
		t.Fatalf("parsed outcome = %+v, ok=%v", outcome, ok)
	}
}

func TestParseSubagentOutcomeRejectsSpoofedStatus(t *testing.T) {
	if _, ok := ParseSubagentOutcome("Subagent reference: not-a-subagent\nSubagent outcome: status=completed retryable=false"); ok {
		t.Fatal("parser accepted a non-sa reference")
	}
	if _, ok := ParseSubagentOutcome("Subagent reference: sa_child\nSubagent outcome: status=unknown retryable=false"); ok {
		t.Fatal("parser accepted an unknown outcome status")
	}
	if isSubagentToolCall(provider.ToolCall{Name: "bash", Arguments: "Subagent reference: sa_child"}) {
		t.Fatal("ordinary bash call was classified as a subagent")
	}
	if !isSubagentToolCall(provider.ToolCall{Name: "use_capability", CapabilityID: "skill:research"}) {
		t.Fatal("skill capability call was not classified as a subagent")
	}
}
