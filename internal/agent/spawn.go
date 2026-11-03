package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"deepzora/internal/event"
	"deepzora/internal/evidence"
	"deepzora/internal/skill"
	"deepzora/internal/tool"
)

const SpawnToolName = "spawn_agents"

type Spawner struct{ runner *SubagentRunner }

func NewSpawner(runner *SubagentRunner) *Spawner { return &Spawner{runner: runner} }
func (*Spawner) Name() string                    { return SpawnToolName }
func (*Spawner) ReadOnly() bool                  { return false }
func (*Spawner) PlanModeSafe() bool              { return true }
func (*Spawner) Description() string {
	return "Run 1–8 independent tasks in fresh subagent sessions and wait for their results. Read-only tasks run concurrently; writable tasks run one at a time. Children cannot delegate and do not see this conversation. Supply self-contained tasks. Optional profile selects a configured subagent; tools only narrows its tools. Plan mode always forces read-only. Results include execution status, a bounded answer preview and a reference for read_subagent_result. There is no background execution, continuation, or dependency graph."
}
func (*Spawner) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"object","properties":{"title":{"type":"string","minLength":1},"task":{"type":"string","minLength":1},"profile":{"type":"string"},"read_only":{"type":"boolean"},"tools":{"type":"array","minItems":1,"items":{"type":"string"}}},"required":["title","task"],"additionalProperties":false}}},"required":["tasks"],"additionalProperties":false}`)
}

type spawnTask struct {
	Title    string          `json:"title"`
	Task     string          `json:"task"`
	Profile  string          `json:"profile,omitempty"`
	ReadOnly bool            `json:"read_only,omitempty"`
	Tools    json.RawMessage `json:"tools,omitempty"`
}

type spawnResult struct {
	Title     string                `json:"title"`
	Status    SubagentOutcomeStatus `json:"status"`
	Ref       string                `json:"ref,omitempty"`
	Head      string                `json:"head,omitempty"`
	More      bool                  `json:"more"`
	Read      []string              `json:"read,omitempty"`
	Wrote     []string              `json:"wrote,omitempty"`
	PathsMore bool                  `json:"paths_more,omitempty"`
	ErrorCode string                `json:"error_code,omitempty"`
	Error     string                `json:"error,omitempty"`
	Retryable bool                  `json:"retryable"`
}

func (s *Spawner) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	tasks, specs, err := s.prepareTasks(ctx, args)
	if err != nil {
		return "", err
	}
	results := make([]spawnResult, len(specs))
	parentID, sink, _, _ := CallContext(ctx)
	if sink == nil {
		sink = event.Discard
	}
	merger := newSubagentProgressMerger(realProgressClock{}, sink, parentID)
	defer merger.Close()
	ctx = withSubagentProgressMerger(ctx, merger)
	merger.directStatus(parentID, subagentPhaseRunning)
	defer func() { finishSpawnGroup(merger, parentID, ctx, results) }()
	var wg sync.WaitGroup
	previousWriter := make(chan struct{})
	close(previousWriter)
	for i, spec := range specs {
		var before, after chan struct{}
		if s.runner.writableRun(spec) {
			before, after = previousWriter, make(chan struct{})
			previousWriter = after
		}
		spec.Sched.afterWriter = before
		wg.Go(func() {
			if after != nil {
				defer func() {
					// A cancelled queued writer still preserves its predecessor's
					// turn; later writers must not leapfrog a slow starting child.
					<-before
					close(after)
				}()
			}
			res := spawnResult{Title: boundedInline(tasks[i].Title, 160), Status: SubagentOutcomeFailed}
			defer func() {
				if problem := recover(); problem != nil {
					res.Status = SubagentOutcomeFailed
					res.ErrorCode = "subagent_error"
					res.Error = fmt.Sprintf("subagent panic: %v", problem)
				}
				results[i] = res
			}()
			id := fmt.Sprintf("%s/sub-%d", parentID, i+1)
			itemCtx := withCallContext(ctx, id, subSinkFor(id, sink), nil, PlanModeFromContext(ctx))
			ledger := evidence.NewLedger()
			itemCtx = evidence.WithLedger(itemCtx, ledger)
			dispatchArgs, _ := json.Marshal(map[string]string{"description": tasks[i].Title, "prompt": tasks[i].Task})
			sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: id, ParentID: parentID, Name: "subagent", Args: string(dispatchArgs), ReadOnly: spec.Grant.ReadOnly}})
			out, err := s.runner.RunProfileSpec(itemCtx, spec)
			answer, ref := splitSubagentRunResult(out)
			res.Ref, res.Status = ref, SubagentOutcomeCompleted
			if err != nil {
				res.ErrorCode, res.Status, res.Retryable = subagentErrorDisposition(err)
				res.Error = boundedInline(err.Error(), 256)
				if outcome := subagentOutcomeFromError(err); outcome != nil {
					res.Ref, answer = outcome.Ref, outcome.FinalAnswer
				}
			}
			answer, _ = splitHostReceipts(answer)
			res.Head = utf8Prefix(answer, min(2000, 8000/len(specs)))
			res.More = len(res.Head) < len(answer)
			summary := ledger.Summary()
			for _, receipt := range summary.Receipts {
				if !receipt.Success {
					continue
				}
				if receipt.Read {
					res.Read = append(res.Read, receipt.Paths...)
				}
				if receipt.Write || receipt.Mutation {
					res.Wrote = append(res.Wrote, receipt.Paths...)
				}
			}
			slices.Sort(res.Read)
			res.Read = slices.Compact(res.Read)
			slices.Sort(res.Wrote)
			res.Wrote = slices.Compact(res.Wrote)
			if parent, ok := evidence.FromContext(ctx); ok {
				parent.MergeChild(summary)
			}
			sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
				ID: id, ParentID: parentID, Name: "subagent", Output: res.Head, Err: res.Error,
				SubagentRef: res.Ref, SubagentStatus: string(res.Status),
				SubagentErrorCode: res.ErrorCode, SubagentRetryable: res.Retryable,
			}})
		})
	}
	wg.Wait()
	// Account for JSON escaping as well as text bytes. Status and references
	// always survive the provider output guard; detail remains in the archive.
	for i := range results {
		r := &results[i]
		r.Title = boundedJSONText(r.Title, 256)
		r.Error = boundedJSONText(r.Error, 512)
		previousLength := len(r.Head)
		r.Head = boundedJSONText(r.Head, 8000/len(results))
		if len(r.Head) < previousLength {
			r.More = true
		}
		var truncated bool
		r.Read, truncated = boundedPathList(r.Read, 4000/len(results))
		r.PathsMore = truncated
		r.Wrote, truncated = boundedPathList(r.Wrote, 4000/len(results))
		r.PathsMore = r.PathsMore || truncated
	}
	out, err := json.Marshal(map[string]any{"agents": results})
	if err != nil {
		return "", err
	}
	return string(out), ctx.Err()
}

func finishSpawnGroup(merger *subagentProgressMerger, id string, ctx context.Context, results []spawnResult) {
	phase := subagentPhaseCompleted
	completed, cancelled := 0, 0
	for _, result := range results {
		if result.Status == SubagentOutcomeCompleted {
			completed++
		}
		if result.Status == SubagentOutcomeCancelled {
			cancelled++
		}
	}
	switch {
	case ctx.Err() != nil || cancelled == len(results):
		phase = subagentPhaseCancelled
	case completed == 0:
		phase = subagentPhaseFailed
	case completed < len(results):
		phase = subagentPhasePartial
	}
	merger.directStatus(id, phase)
}

func boundedJSONText(s string, budget int) string {
	for {
		raw, _ := json.Marshal(s)
		if len(raw) <= budget {
			return s
		}
		s = utf8Prefix(s, len(s)*3/4)
	}
}

func boundedPathList(paths []string, budget int) ([]string, bool) {
	var out []string
	for _, path := range paths {
		raw, _ := json.Marshal(path)
		if len(raw)+1 > budget {
			return out, true
		}
		out = append(out, path)
		budget -= len(raw) + 1
	}
	return out, false
}

func (t *SubagentRunner) preflight(ctx context.Context, spec *ProfileExecSpec) error {
	if _, err := t.nextSubagentDepth(ctx); err != nil {
		return err
	}
	if spec.Worker.UseProfilePrompt && strings.TrimSpace(spec.Worker.SystemPrompt) == "" {
		return fmt.Errorf("profile system prompt is empty")
	}
	spec.Grant.ReadOnly = spec.Grant.ReadOnly || forceReadOnlySpawn(ctx)
	// Validate profile literals before filtering. A typo alongside valid tools
	// must not silently produce a different permission set. Dynamic MCP names
	// retain their existing runtime authorization and discovery boundary.
	known := tool.KnownToolNames()
	if t.parentReg != nil {
		known = append(known, t.parentReg.Names()...)
	}
	for _, diagnostic := range skill.CheckToolReferences([]skill.Skill{{Name: spec.Worker.Name, AllowedTools: normalizeSubagentShellNames(t.parentReg, spec.Grant.ProfileTools)}}, skill.ToolReferenceOptions{Known: known}) {
		if diagnostic.Severity == "warning" {
			return fmt.Errorf("%s", diagnostic.Message)
		}
	}
	names, err := IntersectToolLists(t.parentReg, spec.Grant.ProfileTools, spec.Grant.CallTools)
	if err != nil {
		return err
	}
	reg, _, err := t.buildSubagentRegistry(*spec, names, 1)
	if err != nil {
		return err
	}
	if reg.Len() == 0 {
		return fmt.Errorf("subagent has no permitted tools")
	}
	if !spec.Grant.ReadOnly {
		writer := false
		for _, name := range reg.Names() {
			tl, _ := reg.Get(name)
			writer = writer || !tl.ReadOnly() || name == "use_capability"
		}
		spec.Grant.ReadOnly = !writer
	}
	_, _, _, err = t.resolveSubSessionRuntime(spec.Worker.Model, spec.Worker.Effort)
	return err
}

func (t *SubagentRunner) RunSkill(ctx context.Context, sk skill.Skill, task string, opts skill.SubagentRunOptions) (string, error) {
	if opts.ContinueFrom != "" {
		return "", fmt.Errorf("continue_from is retired; start a fresh subagent")
	}
	spec := ProfileExecSpec{
		Task:   TaskSpec{Objective: task, Description: sk.Name},
		Worker: WorkerSpec{Kind: "skill", Name: sk.Name, Profile: sk.Name, SystemPrompt: sk.Body, UseProfilePrompt: true},
		Grant:  CapabilityGrant{ReadOnly: sk.ReadOnly || PlanModeFromContext(ctx), ProfileTools: sk.AllowedTools},
	}
	cm, ce := "", ""
	if t.profileConfigModel != nil {
		cm = t.profileConfigModel(sk.Name)
	}
	if t.profileConfigEffort != nil {
		ce = t.profileConfigEffort(sk.Name)
	}
	defaultModel, defaultEffort := t.subagentModel, t.subagentEffort
	if t.hasProfileDefaults {
		defaultModel, defaultEffort = t.profileDefaultModel, t.profileDefaultEffort
	}
	spec.Worker.Model, spec.Worker.Effort = ResolveModelEffort(cm, ce, "", "", sk.Model, sk.Effort, defaultModel, defaultEffort)
	if !spec.Grant.ReadOnly {
		spec.Grant.WritePaths = WritePathSet{WholeWorkspace: true, WorkspaceRoot: t.workspaceRoot}
	}
	if err := t.preflight(ctx, &spec); err != nil {
		return "", err
	}
	return t.RunProfileSpec(ctx, spec)
}

func (s *Spawner) prepareTasks(ctx context.Context, args json.RawMessage) ([]spawnTask, []ProfileExecSpec, error) {
	if s == nil || s.runner == nil {
		return nil, nil, fmt.Errorf("subagents are unavailable")
	}
	if SubagentDepth(ctx) > 0 {
		return nil, nil, fmt.Errorf("subagents cannot delegate")
	}
	var p struct {
		Tasks []spawnTask `json:"tasks"`
	}
	d := json.NewDecoder(bytes.NewReader(args))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return nil, nil, fmt.Errorf("invalid spawn_agents arguments: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, nil, fmt.Errorf("expected one argument object")
	}
	if len(p.Tasks) < 1 || len(p.Tasks) > 8 {
		return nil, nil, fmt.Errorf("spawn_agents requires 1–8 tasks")
	}
	specs := make([]ProfileExecSpec, len(p.Tasks))
	for i, item := range p.Tasks {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Task) == "" {
			return nil, nil, fmt.Errorf("task %d requires title and task", i+1)
		}
		var names []string
		if item.Tools != nil {
			if err := json.Unmarshal(item.Tools, &names); err != nil || len(names) == 0 {
				return nil, nil, fmt.Errorf("task %d tools must be a nonempty array", i+1)
			}
			for _, name := range names {
				if _, ok := s.runner.parentReg.Get(name); !ok {
					return nil, nil, fmt.Errorf("task %d: unknown tool %q", i+1, name)
				}
			}
		}
		spec, err := s.runner.buildTaskSpec(ctx, item.Task, item.Title, item.Profile, nil, names, 0, "", "", "", false, item.ReadOnly)
		if err == nil {
			err = s.runner.preflight(ctx, &spec)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		specs[i] = spec
	}
	return p.Tasks, specs, nil
}
