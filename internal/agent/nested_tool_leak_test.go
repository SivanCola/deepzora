package agent

import (
	"testing"

	"deepzora/internal/event"
	"deepzora/internal/jobs"
	"deepzora/internal/provider"
	"deepzora/internal/testutil"
)

func TestSubagentRunnerEventsStayNestedUnderParentCall(t *testing.T) {
	finalText := []provider.Chunk{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}}
	sub := &scriptedProvider{name: "sub", turns: [][]provider.Chunk{
		{testutil.ToolCallChunk("c1", "bash", `{"command":"ls"}`), {Type: provider.ChunkDone}},
		finalText, finalText,
	}}
	task := NewSubagentRunner(SubagentRunnerOptions{Provider: sub, Pricing: nil, ParentRegistry: evidenceRegistry(), MaxSteps: 20, ContextWindow: 0, CompactRatio: 0, Temperature: 0.0, ArchiveDir: "", SysPrompt: "sys", Gate: nil, SubagentModel: "", SubagentEffort: "", ResolveProvider: nil}).
		WithTranscripts(newTestSubagentStore(t, t.TempDir()), t.TempDir(), "base-model", "base-effort")
	jm := jobs.NewManager(event.Discard)
	defer jm.Close()
	rec := &recordSink{}
	ctx := withCallContext(testTaskContext(), "task-call", rec, nil, false)
	ctx = jobs.WithSession(ctx, "parent-session")
	ctx = jobs.WithManager(ctx, jm)

	_, err := task.Execute(ctx, []byte(`{"prompt":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	seen := 0
	for _, e := range rec.evs {
		if e.Tool.ID == "" || e.Tool.ID == "task-call" {
			continue
		}
		seen++
		if e.Tool.ParentID != "task-call" {
			t.Errorf("event kind %v tool %q id %q has ParentID %q, want task-call", e.Kind, e.Tool.Name, e.Tool.ID, e.Tool.ParentID)
		}
	}
	if seen == 0 {
		t.Fatal("no sub-agent tool events observed")
	}
}
