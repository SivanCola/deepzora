package trajectory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepzora/internal/event"
	"deepzora/internal/evidence"
)

type capabilitySink struct {
	events       []event.Event
	anchorSafety []event.AnchorSafetyAudit
	recoveries   []event.ProtocolRecoveryAudit
	outcomes     []evidence.OutcomeSample
	workspace    []event.WorkspaceMutation
	runBudgets   []event.RunBudgetSample
	turns        int
}

func (s *capabilitySink) Emit(e event.Event) { s.events = append(s.events, e) }

func (s *capabilitySink) RecordAnchorSafetyAudit(a event.AnchorSafetyAudit) {
	s.anchorSafety = append(s.anchorSafety, a)
}
func (s *capabilitySink) RecordProtocolRecovery(a event.ProtocolRecoveryAudit) {
	s.recoveries = append(s.recoveries, a)
}
func (s *capabilitySink) RecordTurnCompletion() { s.turns++ }
func (s *capabilitySink) RecordOutcomeProgress(sample evidence.OutcomeSample) {
	s.outcomes = append(s.outcomes, sample)
}

func (s *capabilitySink) RecordWorkspaceMutation(m event.WorkspaceMutation) {
	s.workspace = append(s.workspace, m)
}
func (s *capabilitySink) RecordRunBudget(sample event.RunBudgetSample) {
	s.runBudgets = append(s.runBudgets, sample)
}

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trajectory: %v", err)
	}
	var out []Record
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad record %q: %v", line, err)
		}
		out = append(out, r)
	}
	return out
}

func TestCurrentAuditContractAndForwarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.trajectory.jsonl")
	inner := &capabilitySink{}
	r, err := New(inner, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	audit := event.AnchorSafetyAudit{Mode: "shadow", TaskMode: "interactive", RangeLines: 3, ObservationAge: 2, ShadowAllowed: true, Reason: "would_allow_exact_match"}
	r.RecordAnchorSafetyAudit(audit)
	r.RecordDelegationAdmission(event.DelegationAdmissionAudit{Tool: "research", Verdict: "allow", Reason: "model_decides"})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	records := readRecords(t, path)
	if len(records) != 2 || records[0].AnchorSafetyAudit == nil || !records[0].AnchorSafetyAudit.ShadowAllowed || records[0].AnchorSafetyAudit.ObservationAge != 2 || records[1].DelegationAdmission == nil || records[1].DelegationAdmission.Reason != "model_decides" {
		t.Fatalf("current audit records = %+v", records)
	}
	if len(inner.anchorSafety) != 1 || inner.anchorSafety[0] != audit {
		t.Fatalf("forwarded anchor audits = %+v", inner.anchorSafety)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{`"legacy_allowed"`, `"intent"`} {
		if strings.Contains(string(data), retired) {
			t.Fatalf("trajectory serialized retired audit field %s", retired)
		}
	}
}

func TestRecorderAppendsOrderedTimestampedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.trajectory.jsonl")
	inner := &capabilitySink{}
	now := time.UnixMilli(1754500000000)
	r, err := New(inner, path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "c1", Name: "bash", Args: `{"command":"ls"}`}})
	now = now.Add(120 * time.Millisecond)
	r.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
		ID: "c1", Name: "bash", Output: "ok", DurationMs: 100,
		StartedAt: 1754500000010, EndedAt: 1754500000110,
	}})
	r.Emit(event.Event{Kind: event.Reasoning, Text: "thinking about the next step"})
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	recs := readRecords(t, path)
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3", len(recs))
	}
	for i, rec := range recs {
		if rec.Seq != uint64(i+1) {
			t.Errorf("record %d seq = %d, want %d", i, rec.Seq, i+1)
		}
		if rec.SchemaVersion != SchemaVersion {
			t.Errorf("record %d schema = %d, want %d", i, rec.SchemaVersion, SchemaVersion)
		}
		if rec.Event == nil {
			t.Fatalf("record %d has no event payload", i)
		}
	}
	if recs[0].TS != 1754500000000 || recs[1].TS != 1754500000120 {
		t.Errorf("timestamps = %d, %d, want recorder-clock values", recs[0].TS, recs[1].TS)
	}
	if recs[1].Event.Tool == nil || recs[1].Event.Tool.StartedAt != 1754500000010 || recs[1].Event.Tool.EndedAt != 1754500000110 {
		t.Errorf("tool result record lost execution bounds: %+v", recs[1].Event.Tool)
	}
	if recs[2].Event.Kind != "reasoning" || recs[2].Event.Text != "thinking about the next step" {
		t.Errorf("reasoning record = %+v", recs[2].Event)
	}
	if len(inner.events) != 3 {
		t.Errorf("inner sink saw %d events, want 3", len(inner.events))
	}
}

func TestOutcomeProgressRunwayIsAdditiveAndPresenceAware(t *testing.T) {
	var old Record
	if err := json.Unmarshal([]byte(`{"schema_version":1,"outcome_progress":{"round":1}}`), &old); err != nil {
		t.Fatalf("decode old record: %v", err)
	}
	if old.OutcomeProgress == nil || old.OutcomeProgress.Runway != nil {
		t.Fatalf("old record runway = %+v, want unobserved nil", old.OutcomeProgress)
	}

	zero := 0
	data, err := json.Marshal(Record{
		SchemaVersion:   SchemaVersion,
		OutcomeProgress: &OutcomeProgress{Round: 1, Runway: &zero, RunwaySpent: true},
	})
	if err != nil {
		t.Fatalf("encode new record: %v", err)
	}
	if !strings.Contains(string(data), `"runway":0`) {
		t.Fatalf("zero balance was omitted: %s", data)
	}
	// A previous reader ignores the additive fields and keeps its known data.
	var legacy struct {
		OutcomeProgress *struct {
			Round int `json:"round"`
		} `json:"outcome_progress"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil || legacy.OutcomeProgress == nil || legacy.OutcomeProgress.Round != 1 {
		t.Fatalf("legacy decode = %+v, %v", legacy, err)
	}
}

func TestRecorderForwardsAfterCloseWithoutRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.trajectory.jsonl")
	inner := &capabilitySink{}
	r, err := New(inner, path, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.Emit(event.Event{Kind: event.Text, Text: "before"})
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	r.Emit(event.Event{Kind: event.Text, Text: "after"})

	if len(readRecords(t, path)) != 1 {
		t.Fatalf("post-close event must not be recorded")
	}
	if len(inner.events) != 2 {
		t.Fatalf("inner sink saw %d events, want 2 (forwarding survives Close)", len(inner.events))
	}
}

func TestNewFailsOnUnwritablePath(t *testing.T) {
	if _, err := New(event.Discard, filepath.Join(t.TempDir(), "missing", "run.jsonl"), nil); err == nil {
		t.Fatal("New must fail when the parent directory does not exist")
	}
}
