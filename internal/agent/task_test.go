package agent

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"deepzora/internal/event"
	"deepzora/internal/evidence"
	"deepzora/internal/jobs"
	"deepzora/internal/provider"
	"deepzora/internal/testutil"
	"deepzora/internal/tool"
)

func testTaskContext() context.Context {
	return WithParentSession(context.Background(), "parent-session")
}

// TestSubagentRunnerReturnsSubAgentFinalAnswer runs a task against a mock provider
// that emits a single text turn, and verifies the tool returns that text with a
// transcript reference — sub-agent intermediate state isn't supposed to leak.
func TestSubagentRunnerReturnsSubAgentFinalAnswer(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "found 3 callers of Foo"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	task := newTestSubagentRunner(t, sub, parentReg, "test-sys-prompt", "", "", nil)

	out, err := task.Execute(testTaskContext(), []byte(`{"prompt":"find callers of Foo"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	_ = subagentRefFromOutput(t, out)
	if !strings.Contains(out, "found 3 callers of Foo") {
		t.Errorf("got %q, want sub-agent final answer", out)
	}
	if !strings.Contains(out, "Read the retained result with read_subagent_result.") {
		t.Errorf("got %q, want continuation guidance", out)
	}

	// The sub-agent must have received the prompt as its user message and
	// the configured system prompt at the top — proving the session was
	// fresh, not the parent's.
	if sys := sub.lastReq.Messages[0]; sys.Role != provider.RoleSystem || sys.Content != "test-sys-prompt" {
		t.Errorf("first message = %+v, want system 'test-sys-prompt'", sys)
	}
	if got := lastUser(sub.lastReq); !strings.Contains(got, `<subagent-context event="SubagentStart">`) || !strings.Contains(got, "find callers of Foo") {
		t.Errorf("sub-agent user = %q, want SubagentStart context plus prompt", got)
	}
}

func TestSubagentRunnerInjectsWorkspaceContextIntoSubagentPrompt(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "answer"},
		{Type: provider.ChunkDone},
	}}
	workspace := t.TempDir()
	task := NewSubagentRunner(SubagentRunnerOptions{Provider: sub, Pricing: nil, ParentRegistry: tool.NewRegistry(), MaxSteps: 20, ContextWindow: 0, CompactRatio: 0, Temperature: 0.0, ArchiveDir: "", SysPrompt: "sys", Gate: nil, SubagentModel: "", SubagentEffort: "", ResolveProvider: nil}).
		WithTranscripts(newTestSubagentStore(t, t.TempDir()), workspace, "base-model", "base-effort")

	if _, err := task.Execute(testTaskContext(), []byte(`{"prompt":"inspect project"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sys := sub.lastReq.Messages[0]; sys.Role != provider.RoleSystem || sys.Content != "sys" {
		t.Fatalf("system prompt = %+v, want original prompt", sys)
	}
	got := lastUser(sub.lastReq)
	if !strings.Contains(got, `<workspace-context event="SubagentWorkspace">`) ||
		!strings.Contains(got, "Current workspace: "+strconv.Quote(workspace)) ||
		!strings.Contains(got, `prefer "." or relative paths`) ||
		!strings.Contains(got, "inspect project") {
		t.Fatalf("sub-agent user = %q, want workspace context plus prompt", got)
	}
}

func TestSubagentRunnerInheritsReasoningLanguageFromContext(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "done"},
		{Type: provider.ChunkDone},
	}}
	task := newTestSubagentRunner(t, sub, tool.NewRegistry(), "sys", "", "", nil)

	ctx := WithReasoningLanguagePreference(testTaskContext(), "zh")
	if _, err := task.Execute(ctx, []byte(`{"prompt":"inspect auth"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := lastUser(sub.lastReq)
	if !strings.HasPrefix(got, "<reasoning-language>") || !strings.Contains(got, "简体中文") || !strings.Contains(got, "inspect auth") {
		t.Fatalf("sub-agent user = %q, want reasoning-language-prefixed prompt", got)
	}
}

func TestSubagentRunnerPropagatesSubagentImageCandidates(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "image received"},
		{Type: provider.ChunkDone},
	}}
	task := newTestSubagentRunner(t, sub, tool.NewRegistry(), "sys", "", "", nil)
	ctx := WithSubagentImageCandidates(testTaskContext(), []string{"data:image/png;base64,AAAA"})
	if _, err := task.Execute(ctx, []byte(`{"prompt":"inspect the attached image"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var images []string
	for _, msg := range sub.lastReq.Messages {
		if msg.Role == provider.RoleUser {
			images = msg.Images
		}
	}
	if len(images) != 1 || images[0] != "data:image/png;base64,AAAA" {
		t.Fatalf("sub-agent images = %v, want the parent candidate", images)
	}
}

// TestSubagentRunnerFiltersTools verifies the whitelist behaviour: when the caller
// names a subset of tools, the sub-agent's registry contains exactly that set
// with recursive delegation tools available while max_subagent_depth leaves one
// more layer.
func TestSubagentRunnerFiltersTools(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "ok"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "read_file", readOnly: true})
	parentReg.Add(fakeTool{name: "write_file", readOnly: false})
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	task := newTestSubagentRunner(t, sub, parentReg, "sys", "", "", nil)
	parentReg.Add(NewSpawner(task)) // simulate the wiring in cli.setup
	parentReg.Add(fakeTool{name: "run_skill", readOnly: false})
	parentReg.Add(fakeTool{name: "read_only_skill", readOnly: true})
	parentReg.Add(fakeTool{name: "research", readOnly: false})

	args := []byte(`{"prompt":"x","tools":["read_file","task","write_file","run_skill","read_only_skill","research"]}`)
	if _, err := task.Execute(testTaskContext(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// The sub-agent's tool schemas should reflect the whitelist minus always
	// unavailable background/install tools. Recursive tools stay visible at depth 1.
	got := map[string]bool{}
	for _, s := range sub.lastReq.Tools {
		got[s.Name] = true
	}
	for _, want := range []string{"read_file", "write_file"} {
		if !got[want] {
			t.Errorf("sub-agent tools = %v, want %q exposed at depth 1", got, want)
		}
	}
	if got["bash"] {
		t.Errorf("sub-agent tools = %v, want bash omitted when not requested", got)
	}
}

// TestSubagentRunnerDefaultsToParentToolsWithDepthRemaining covers the no-whitelist
// path: the first-layer sub-agent inherits parent tools except always-hidden
// background/install tools because it still has one delegation layer available.
func TestSubagentRunnerDefaultsToParentToolsWithoutMetaTools(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "ok"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "read_file", readOnly: true})
	parentReg.Add(fakeTool{name: "grep", readOnly: true})
	task := newTestSubagentRunner(t, sub, parentReg, "sys", "", "", nil)
	parentReg.Add(NewSpawner(task))
	parentReg.Add(fakeTool{name: "run_skill", readOnly: false})
	parentReg.Add(fakeTool{name: "read_only_skill", readOnly: true})
	parentReg.Add(fakeTool{name: "explore", readOnly: false})
	parentReg.Add(fakeTool{name: "research", readOnly: false})
	parentReg.Add(fakeTool{name: "review", readOnly: false})
	parentReg.Add(fakeTool{name: "security_review", readOnly: false})
	parentReg.Add(fakeTool{name: "remember", readOnly: false})

	if _, err := task.Execute(testTaskContext(), []byte(`{"prompt":"x"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := map[string]bool{}
	for _, s := range sub.lastReq.Tools {
		got[s.Name] = true
	}
	for _, want := range []string{"read_file", "grep", "remember"} {
		if !got[want] {
			t.Errorf("default sub-agent tools = %v, want %q inherited at depth 1", got, want)
		}
	}
}

func TestSubagentRunnerMaxSubagentDepthOneRestoresSingleLayerBoundary(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "single layer answer"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "read_file", readOnly: true})
	task := newTestSubagentRunner(t, sub, parentReg, "sys", "", "", nil)
	parentReg.Add(NewSpawner(task))
	parentReg.Add(fakeTool{name: "run_skill", readOnly: false})
	parentReg.Add(fakeTool{name: "read_only_task", readOnly: true})

	if _, err := task.Execute(testTaskContext(), []byte(`{"prompt":"single layer"}`)); err != nil {
		t.Fatalf("root task should still spawn first-layer subagent: %v", err)
	}
	got := map[string]bool{}
	for _, s := range sub.lastReq.Tools {
		got[s.Name] = true
	}
	for _, hidden := range []string{"task", "run_skill", "read_only_task"} {
		if got[hidden] {
			t.Fatalf("max_subagent_depth=1 should hide recursive tool %q; tools=%v", hidden, toolSchemaNames(sub.lastReq.Tools))
		}
	}

	depthOneCtx := WithSubagentDepth(testTaskContext(), 1)
	if _, err := task.Execute(depthOneCtx, []byte(`{"prompt":"too deep"}`)); err == nil || !strings.Contains(err.Error(), "subagents cannot delegate") {
		t.Fatalf("depth-1 task error = %v, want max depth rejection", err)
	}
}

func TestSubagentRunnerUsesConfiguredProfileForExecution(t *testing.T) {
	parent := &mockProvider{name: "parent", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "parent answer"},
		{Type: provider.ChunkDone},
	}}
	resolved := &mockProvider{name: "resolved", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "resolved answer"},
		{Type: provider.ChunkDone},
	}}
	var gotModel, gotEffort string
	resolve := func(model, effort string) (provider.Provider, *provider.Pricing, int, error) {
		gotModel, gotEffort = model, effort
		return resolved, nil, 0, nil
	}
	task := newTestSubagentRunner(t, parent, tool.NewRegistry(), "sys", "deepseek-pro", "max", resolve)

	out, err := task.Execute(testTaskContext(), []byte(`{"prompt":"x"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "resolved answer") {
		t.Fatalf("sub-agent did not use resolved provider, got %q", out)
	}
	if gotModel != "deepseek-pro" || gotEffort != "max" {
		t.Fatalf("resolved profile = %q/%q, want deepseek-pro/max", gotModel, gotEffort)
	}
}

func TestSubagentRunnerReturnsProfileResolutionErrors(t *testing.T) {
	parent := &mockProvider{name: "parent", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "parent answer"},
		{Type: provider.ChunkDone},
	}}
	resolve := func(string, string) (provider.Provider, *provider.Pricing, int, error) {
		return nil, nil, 0, errors.New("bad effort")
	}
	task := newTestSubagentRunner(t, parent, tool.NewRegistry(), "sys", "", "", resolve)

	_, err := task.Execute(testTaskContext(), []byte(`{"prompt":"x","effort":"turbo"}`))
	if err == nil || !strings.Contains(err.Error(), "bad effort") {
		t.Fatalf("Execute error = %v, want profile resolution error", err)
	}
}

func TestSubagentRunnerRequiresTranscriptStore(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "answer"},
		{Type: provider.ChunkDone},
	}}
	task := NewSubagentRunner(SubagentRunnerOptions{Provider: sub, Pricing: nil, ParentRegistry: tool.NewRegistry(), MaxSteps: 20, ContextWindow: 0, CompactRatio: 0, Temperature: 0.0, ArchiveDir: "", SysPrompt: "sys", Gate: nil, SubagentModel: "", SubagentEffort: "", ResolveProvider: nil})

	_, err := task.Execute(testTaskContext(), []byte(`{"prompt":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "transcript store is required") {
		t.Fatalf("Execute error = %v, want transcript store requirement", err)
	}
}

// TestSubagentRunnerRunsEphemerallyWithoutParentSession mirrors headless `deepzora run`:
// the store is wired but the context carries no parent session, so the sub-agent
// must run without persistence and return its plain answer (no transcript ref).
func TestSubagentRunnerRunsEphemerallyWithoutParentSession(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "headless answer"},
		{Type: provider.ChunkDone},
	}}
	task := newTestSubagentRunner(t, sub, tool.NewRegistry(), "sys", "", "", nil)

	out, err := task.Execute(context.Background(), []byte(`{"prompt":"x"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "headless answer") {
		t.Fatalf("got %q, want sub-agent final answer", out)
	}
	if strings.Contains(out, "Subagent reference") {
		t.Fatalf("ephemeral run should not emit a transcript reference: %q", out)
	}
}

func TestReadOnlySubagentRunnerRunsEphemerallyWithReadOnlyRegistry(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "read-only findings"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "read_file", readOnly: true})
	parentReg.Add(fakeTool{name: "write_file", readOnly: false})
	parentReg.Add(fakeTool{name: "todo_write", readOnly: true})
	parentReg.Add(fakeTool{name: "connect_tool_source", readOnly: true})
	parentReg.Add(fakeTool{name: "read_only_skill", readOnly: true})
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	task := newTestSubagentRunner(t, sub, parentReg, "writer sys", "", "", nil)
	readonly := NewReadOnlySubagentRunner(task)
	parentReg.Add(NewSpawner(task))
	parentReg.Add(fakeTool{name: "read_only_task", readOnly: true})

	out, err := readonly.Execute(testTaskContext(), []byte(`{"prompt":"inspect callers"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "read-only findings") {
		t.Fatalf("output = %q, want final answer", out)
	}
	if strings.Contains(out, "Subagent reference") {
		t.Fatalf("read_only_task should not persist transcript refs: %q", out)
	}
	if sys := sub.lastReq.Messages[0]; sys.Role != provider.RoleSystem || sys.Content != DefaultReadOnlyTaskSystemPrompt {
		t.Fatalf("read_only_task system prompt = %+v, want read-only prompt", sys)
	}
	if got := lastUser(sub.lastReq); !strings.Contains(got, "Current workspace: ") || !strings.Contains(got, "inspect callers") {
		t.Fatalf("read_only_task user = %q, want workspace context plus prompt", got)
	}

	got := map[string]bool{}
	for _, s := range sub.lastReq.Tools {
		got[s.Name] = true
	}
	for _, want := range []string{"read_file", "bash"} {
		if !got[want] {
			t.Fatalf("read_only_task sub-agent missing %q; tools=%v", want, toolSchemaNames(sub.lastReq.Tools))
		}
	}
	for _, hidden := range []string{"write_file", "todo_write", "connect_tool_source", "task"} {
		if got[hidden] {
			t.Fatalf("read_only_task sub-agent should hide %q; tools=%v", hidden, toolSchemaNames(sub.lastReq.Tools))
		}
	}
	for _, want := range []string{"read_only_task", "read_only_skill"} {
		if got[want] {
			t.Fatalf("read_only_task depth-1 sub-agent should hide %q; tools=%v", want, toolSchemaNames(sub.lastReq.Tools))
		}
	}
}

func TestSubagentRunnerRejectsContinuationWithoutParentSession(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "answer"},
		{Type: provider.ChunkDone},
	}}
	task := newTestSubagentRunner(t, sub, tool.NewRegistry(), "sys", "", "", nil)

	_, err := task.Execute(context.Background(), []byte(`{"prompt":"x","continue_from":"sa_whatever"}`))
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("Execute error = %v, want persisted-session requirement", err)
	}
}

// startTerminalBackgroundMutation registers a background task job that publishes
// one mutation and returns after it reaches a terminal state, ready to collect.
func startTerminalBackgroundMutation(t *testing.T, jm *jobs.Manager, session, path string) string {
	t.Helper()
	j := jm.StartForSession(session, "task", "bg writer", func(ctx context.Context, _ io.Writer) (string, error) {
		jobs.PublishEvidence(ctx, evidence.ChildEvidenceSummary{Receipts: []evidence.Receipt{{
			ToolName: "write_file", Success: true, Write: true, Mutation: true, Paths: []string{path},
		}}})
		return "background answer", nil
	})
	if res := awaitTaskJobs(t, jm, session, []string{j.ID}); len(res) != 1 || res[0].Status != jobs.Done {
		t.Fatalf("background job = %+v, want done", res)
	}
	return j.ID
}

func waitBuiltin(t *testing.T, reg *tool.Registry) {
	t.Helper()
	wait, ok := tool.LookupBuiltin("job_output")
	if !ok {
		t.Fatal("wait builtin not registered")
	}
	reg.Add(wait)
}

func TestBackgroundEvidenceNotCommittedWhenTurnFails(t *testing.T) {
	// The delivery turn collects a background writer's mutation via wait, then
	// fails to sign it off, exhausting readiness. Because the turn never
	// delivered, the lease must not be committed: the mutation stays collectable
	// so the next turn can review it instead of shipping it unreviewed.
	jm := jobs.NewManager(event.Discard)
	defer jm.Close()
	jobID := startTerminalBackgroundMutation(t, jm, "parent-session", "internal/auth/session.go")

	reg := evidenceRegistry()
	waitBuiltin(t, reg)
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{testutil.ToolCallChunk("w", "job_output", `{"job_id":"`+jobID+`","wait":true}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}}, // no sign-off
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
	}}
	firstCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := New(prov, reg, NewSession(""), Options{Jobs: jm}, event.FuncSink(func(e event.Event) {
		if e.Kind == event.ToolResult && e.Tool.Name == "job_output" {
			cancel()
		}
	}))
	ctx := withClosedLoopContext(jobs.WithManager(WithParentSession(firstCtx, "parent-session"), jm))
	ctx = jobs.WithSession(ctx, "parent-session")

	err := a.Run(ctx, "collect and finish the background task")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("turn = %v, want readiness exhaustion on the uncollected sign-off", err)
	}
	// The failed turn must not have consumed the evidence.
	if leased := jm.LeaseEvidenceForSession("parent-session", jobID); !leased.HasMutation() {
		t.Fatalf("failed delivery turn consumed the background evidence: %+v", leased)
	}
}

func TestBackgroundEvidenceCommittedWhenTurnDelivers(t *testing.T) {
	// A successful turn that collected a background writer's mutation commits the
	// lease, permanently draining the job's evidence so a later re-poll does not
	// re-demand review of work already delivered.
	jm := jobs.NewManager(event.Discard)
	defer jm.Close()
	jobID := startTerminalBackgroundMutation(t, jm, "parent-session", "notes.txt")

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	waitBuiltin(t, reg)
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{testutil.ToolCallChunk("w", "job_output", `{"job_id":"`+jobID+`","wait":true}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "collected the result"}, {Type: provider.ChunkDone}},
	}}
	// No delivery profile: the turn succeeds immediately after collecting, so the
	// commit-on-success hook fires without a full sign-off script.
	a := New(prov, reg, NewSession(""), Options{Jobs: jm}, event.Discard)
	ctx := jobs.WithManager(WithParentSession(context.Background(), "parent-session"), jm)
	ctx = jobs.WithSession(ctx, "parent-session")

	if err := a.Run(ctx, "collect the background task"); err != nil {
		t.Fatalf("delivering turn failed: %v", err)
	}
	if leased := jm.LeaseEvidenceForSession("parent-session", jobID); len(leased.Receipts) != 0 {
		t.Fatalf("delivered turn did not commit the background lease: %+v", leased)
	}
}

// TestFailedTurnBackgroundMutationForcesReadinessOnNextRunWithoutWait extends
// TestBackgroundEvidenceNotCommittedWhenTurnFails: after the first turn collects
// a background mutation via wait but fails to sign it off, Run's Reset wipes the
// per-turn ledger before the second turn starts. Without re-injecting the still
// uncommitted mutation, a second turn that never calls wait/job_output again
// would sail through final-readiness having never seen it. Run must re-lease it
// automatically so the gate still blocks.
func TestFailedTurnBackgroundMutationForcesReadinessOnNextRunWithoutWait(t *testing.T) {
	jm := jobs.NewManager(event.Discard)
	defer jm.Close()
	jobID := startTerminalBackgroundMutation(t, jm, "parent-session", "internal/auth/session.go")

	reg := evidenceRegistry()
	waitBuiltin(t, reg)
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{testutil.ToolCallChunk("w", "job_output", `{"job_id":"`+jobID+`","wait":true}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}}, // no sign-off
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
		// Second Run: the model never calls wait/job_output again.
		{{Type: provider.ChunkText, Text: "sure, here you go"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "sure, here you go"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "sure, here you go"}, {Type: provider.ChunkDone}},
	}}
	firstCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := New(prov, reg, NewSession(""), Options{Jobs: jm}, event.FuncSink(func(e event.Event) {
		if e.Kind == event.ToolResult && e.Tool.Name == "job_output" {
			cancel()
		}
	}))
	ctx := withClosedLoopContext(jobs.WithManager(WithParentSession(firstCtx, "parent-session"), jm))
	ctx = jobs.WithSession(ctx, "parent-session")

	if err := a.Run(ctx, "collect and finish the background task"); !errors.Is(err, context.Canceled) {
		t.Fatalf("first turn = %v, want readiness exhaustion on the uncollected sign-off", err)
	}
	if leased := jm.LeaseEvidenceForSession("parent-session", jobID); !leased.HasMutation() {
		t.Fatalf("first failed turn consumed the background evidence: %+v", leased)
	}

	ctx = withClosedLoopContext(jobs.WithSession(jobs.WithManager(WithParentSession(context.Background(), "parent-session"), jm), "parent-session"))
	if err := a.Run(ctx, "answer directly"); err != nil {
		t.Fatal(err)
	}
	if receipt := a.CompletionReceipt(); receipt == nil || receipt.AssessmentKind != "facts" {
		t.Fatalf("lost background facts: %+v", receipt)
	}
}

func TestRestartRecoversPendingBackgroundMutationForcesReadinessWithoutWait(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")
	first := jobs.NewManager(event.Discard)
	first.BindSession("parent-session", sessionPath)
	j := first.StartForSession("parent-session", "task", "bg writer", func(ctx context.Context, _ io.Writer) (string, error) {
		jobs.PublishEvidence(ctx, evidence.ChildEvidenceSummary{Receipts: []evidence.Receipt{{
			ToolName: "write_file", Success: true, Write: true, Mutation: true, Paths: []string{"internal/auth/session.go"},
		}}})
		return "background answer", nil
	})
	if res := awaitTaskJobs(t, first, "parent-session", []string{j.ID}); len(res) != 1 || res[0].Status != jobs.Done {
		t.Fatalf("background job = %+v, want done", res)
	}
	first.Close() // the process exits before any turn ever leased this evidence

	second := jobs.NewManager(event.Discard)
	defer second.Close()
	second.BindSession("parent-session", sessionPath)

	reg := evidenceRegistry()
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "all set"}, {Type: provider.ChunkDone}},
	}}
	a := New(prov, reg, NewSession(""), Options{Jobs: second}, event.Discard)
	ctx := withClosedLoopContext(jobs.WithManager(WithParentSession(context.Background(), "parent-session"), second))
	ctx = jobs.WithSession(ctx, "parent-session")

	if err := a.Run(ctx, "what's the status?"); err != nil {
		t.Fatalf("post-restart turn = %v, want readiness exhaustion on the recovered mutation", err)
	}
	if leased := second.LeaseEvidenceForSession("parent-session", j.ID); leased.HasMutation() {
		t.Fatalf("recovered evidence lost after the failed post-restart turn: %+v", leased)
	}
}

func subagentRefFromOutput(t *testing.T, out string) string {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if after, ok := strings.CutPrefix(line, "Subagent reference: "); ok {
			return strings.TrimSpace(after)
		}
	}
	t.Fatalf("no subagent reference in output:\n%s", out)
	return ""
}

func TestSubSinkForwardsUsageToParent(t *testing.T) {
	var got []event.Event
	parent := event.FuncSink(func(e event.Event) {
		got = append(got, e)
	})
	subSinkFor("task_1", parent).Emit(event.Event{
		Kind:        event.Usage,
		Usage:       &provider.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		UsageSource: event.UsageSourceSubagent,
	})
	if len(got) != 1 || got[0].Usage == nil || got[0].UsageSource != event.UsageSourceSubagent {
		t.Fatalf("forwarded events = %+v, want subagent usage", got)
	}
}

func newTestSubagentRunner(t *testing.T, prov provider.Provider, reg *tool.Registry, sysPrompt, subagentModel, subagentEffort string, resolve func(string, string) (provider.Provider, *provider.Pricing, int, error)) *SubagentRunner {
	t.Helper()
	return NewSubagentRunner(SubagentRunnerOptions{
		Provider:        prov,
		ParentRegistry:  reg,
		MaxSteps:        20,
		SysPrompt:       sysPrompt,
		SubagentModel:   subagentModel,
		SubagentEffort:  subagentEffort,
		ResolveProvider: resolve,
	}).WithTranscripts(newTestSubagentStore(t, t.TempDir()), t.TempDir(), "base-model", "base-effort")
}

type panicProvider struct{ name string }

func (p panicProvider) Name() string { return p.name }

func (p panicProvider) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	panic("subagent boom")
}
