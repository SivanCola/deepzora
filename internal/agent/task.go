package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"deepzora/internal/ablation"
	"deepzora/internal/checkpoint"
	"deepzora/internal/event"
	"deepzora/internal/evidence"
	"deepzora/internal/imageinput"
	"deepzora/internal/jobs"
	"deepzora/internal/memory"
	"deepzora/internal/permission"
	"deepzora/internal/planmode"
	"deepzora/internal/provider"
	"deepzora/internal/sandbox"
	"deepzora/internal/sessiontemp"
	"deepzora/internal/tool"
	"deepzora/internal/workspacelease"
)

// DefaultTaskSystemPrompt steers a sub-agent toward focused, terse delivery —
// it doesn't see the parent's conversation so it must self-contain.
const DefaultTaskSystemPrompt = `You are a sub-agent invoked by a parent coding agent to carry out one focused task.
Use the provided tools to investigate or act. For MCP, use the stable use_capability
proxy (list → inspect → call); do not expect direct mcp__* tool schemas. Return a
single final answer that is concise and self-contained — the parent will see only
that answer, not your tool calls or reasoning. If you need to ask for clarification,
fail with a precise question instead of guessing.`

// DefaultReadOnlyTaskSystemPrompt steers read-only sub-agents toward isolated
// research. They never receive writer tools, persisted transcript controls, or
// background process controls, so their final answer is the only handoff.
const DefaultReadOnlyTaskSystemPrompt = `You are a read-only research sub-agent invoked by a parent coding agent.
Use only the provided read-only tools to inspect code, docs, history, and safe shell output.
For MCP, use use_capability only for authorized tools that declare readOnly and are
not destructive; never treat missing readOnlyHint as permission to call. Do not
attempt to write files, install capabilities, mutate memory, control long-lived
processes, or delegate to writer-capable agents. Subagents cannot delegate.
Return a concise, self-contained final answer with the evidence the parent needs.`

const subagentStartContext = `<subagent-context event="SubagentStart">
Before acting, check the available skills and tools. If a relevant skill is available, invoke it before continuing. Do not delegate to another sub-agent.
</subagent-context>`

// read_skill renders playbook text inline and cannot delegate.
var subagentRecursiveTools = []string{
	"spawn_agents",
	"task",
	"read_only_task",
	"run_skill",
	"read_only_skill",
	"explore",
	"research",
	"review",
	"security_review",
}

var subagentAlwaysHiddenTools = []string{
	"parallel_tasks",
	"fleet",
	"read_subagent_result",
	"set_session_title",
	"install_skill",
	"install_source",
}

var subagentJobTools = []string{
	"job_output",
	"job_kill",
}

var readOnlySubagentWorkflowTools = []string{
	"connect_tool_source",
}

// AlwaysHiddenSubagentTools returns the tool names excluded from every
// subagent's registry regardless of an explicit allowlist.
// That covers both subagentAlwaysHiddenTools and subagentJobTools —
// SubagentToolRegistryForDepth and its read-only variant strip the job tools
// unconditionally too. Host UIs offering a tool picker for a subagent
// profile's allowed-tools should exclude these from the offered choices —
// selecting them would be silently ignored at runtime.
func AlwaysHiddenSubagentTools() []string {
	names := append([]string(nil), subagentAlwaysHiddenTools...)
	names = append(names, subagentRecursiveTools...)
	return append(names, subagentJobTools...)
}

// SubagentMetaTools returns the tool names that spawned agents should not inherit
// from the parent registry unless a future call site deliberately opts into a
// different boundary. They can spawn or author more agent work, so excluding them
// preserves one layer of delegation without adding a spawn-count cap.
// read_skill stays listed here so the guardian and planner surfaces, which
// exclude these names, keep their provider-visible tool sets byte-identical —
// only the sub-agent depth cap deliberately stopped stripping it.
func SubagentMetaTools() []string {
	out := append([]string(nil), subagentRecursiveTools...)
	out = append(out, "read_skill")
	out = append(out, subagentAlwaysHiddenTools...)
	return out
}

// SubagentToolRegistry returns the tool set exposed inside spawned sub-agents:
// the requested whitelist (or every parent tool), minus meta tools that would
// spawn more agent work and job tools whose runtime manager is not injected into
// sub-agents. When bash is present, it is wrapped to advertise and allow only
// foreground execution.
func SubagentToolRegistry(parent *tool.Registry, names []string) *tool.Registry {
	return SubagentToolRegistryForDepth(parent, names, 1, 1)
}

// SubagentToolRegistryForDepth returns the writer-capable tool set for a spawned
// subagent. Every child is prohibited from further delegation.
//
// Direct mcp__* schemas are never exposed: MCP goes only through the fixed
// use_capability proxy so connect/disconnect/tool-list churn cannot change the
// child provider-visible tool prefix. With no explicit allowlist the child gets
// the full proxy (installed/authorized MCP, including tools without
// readOnlyHint). An explicit allowlist converts mcp__* / mcp-tool: names into a
// capability-id allowlist on a restricted proxy.
func SubagentToolRegistryForDepth(parent *tool.Registry, names []string, childDepth, maxDepth int) *tool.Registry {
	return SubagentToolRegistryForDepthWithRuntime(parent, names, childDepth, maxDepth, nil)
}

// SubagentToolRegistryForDepthWithRuntime is SubagentToolRegistryForDepth with
// an optional session MCP runtime used when the parent registry has no
// use_capability (for example Economy or legacy callers) but sub-agents still
// need the proxy.
func SubagentToolRegistryForDepthWithRuntime(parent *tool.Registry, names []string, childDepth, maxDepth int, runtime *MCPCapabilityRuntime) *tool.Registry {
	exclude := append([]string(nil), subagentAlwaysHiddenTools...)
	if childDepth > 0 {
		exclude = append(exclude, subagentRecursiveTools...)
	}
	exclude = append(exclude, subagentJobTools...)
	sub := FilterRegistry(parent, names, exclude...)
	stripDirectMCPTools(sub)
	attachSubagentCapabilityProxy(parent, sub, names, runtime)
	shellName := "bash"
	if _, ok := sub.Get("pwsh"); ok {
		shellName = "pwsh"
		sub.RemovePrefix("bash")
	}
	if shell, ok := sub.Get(shellName); ok {
		sub.Add(foregroundOnlyBash{inner: shell})
	}
	return sub
}

type foregroundOnlyBash struct {
	inner tool.Tool
}

func (b foregroundOnlyBash) Name() string { return b.inner.Name() }

func (b foregroundOnlyBash) Description() string {
	desc := strings.TrimSpace(b.inner.Description())
	if desc == "" {
		desc = "Execute a command in the shell and return combined stdout/stderr."
	}
	desc = strings.Replace(desc, "Execute a command in the shell", "Execute a foreground command in the shell", 1)
	return desc + " Background execution is unavailable inside subagents."
}

func (b foregroundOnlyBash) Schema() json.RawMessage {
	if b.Name() == "pwsh" {
		return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"PowerShell command to execute in the foreground"},"description":{"type":"string","description":"Clear 5-10 word active-voice description shown in the UI"},"timeout_ms":{"type":"integer","minimum":1}},"required":["command","description"]}`)
	}
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute in the foreground"}},"required":["command"]}`)
}

func (b foregroundOnlyBash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		RunInBackground bool `json:"run_in_background"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.RunInBackground {
		return "", tool.Blocked(fmt.Sprintf("blocked: background %s is unavailable in subagents; run a foreground command or ask the parent agent to start a background job", b.Name()))
	}
	return b.inner.Execute(ctx, args)
}

func (b foregroundOnlyBash) ReadOnly() bool { return b.inner.ReadOnly() }

type readOnlyBash struct {
	inner tool.Tool
}

func (b readOnlyBash) Name() string { return b.inner.Name() }

func (b readOnlyBash) Description() string {
	desc := strings.TrimSpace(b.inner.Description())
	if desc == "" {
		desc = "Execute a command in the shell and return combined stdout/stderr."
	}
	desc = strings.Replace(desc, "Execute a command in the shell", "Execute a foreground read-only command in the shell", 1)
	return desc + " Only permission-classified read-only commands are allowed; shell operators, background execution, process preservation, and write-capable arguments are blocked."
}

func (b readOnlyBash) Schema() json.RawMessage {
	if b.Name() == "pwsh" {
		return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Read-only PowerShell command to execute in the foreground"},"description":{"type":"string","description":"Clear 5-10 word active-voice description shown in the UI"},"timeout_ms":{"type":"integer","minimum":1}},"required":["command","description"]}`)
	}
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Read-only shell command to execute in the foreground. Must match the permission-layer read-only command policy."}},"required":["command"]}`)
}

func (b readOnlyBash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if !permission.BashCommandIsReadOnly(args) {
		return "", tool.Blocked("blocked: read-only subagents can run only permission-classified foreground read-only commands")
	}
	return b.inner.Execute(ctx, args)
}

func (readOnlyBash) ReadOnly() bool { return true }

// SubagentRunner owns execution, cancellation, archival and cleanup for every
// delegation entry point. Each child has a fresh session, restricted tools and
// a bounded budget; only its final text is handed back to the parent.
type SubagentRunner struct {
	imageInput                                *imageinput.Config
	prov                                      provider.Provider
	pricing                                   *provider.Pricing
	quoteContext                              *event.QuoteContext
	parentReg                                 *tool.Registry
	maxSteps                                  int
	contextWindow                             int
	compaction                                compactionThreshold
	temperature                               float64
	archiveDir                                string
	sysPrompt                                 string
	gate                                      Gate
	subagentModel, subagentEffort             string
	profileDefaultModel, profileDefaultEffort string
	hasProfileDefaults                        bool
	resolveProvider                           func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error)
	transcripts                               *SubagentStore
	workspaceRoot                             string
	baseModel                                 string
	baseEffort                                string
	identityProfile                           func(modelRef, effort string) (string, string)
	ablation                                  ablation.Set
	workspaceLease                            *workspacelease.Owner
	// scheduler is the session-scoped concurrency + write-claim controller.
	// Every production entry point uses the same owner.
	scheduler *SubagentScheduler
	// profileLookup resolves profile= names from the live Skill store without
	// embedding the name list in the tool schema (cache stability).
	profileLookup ProfileLookup
	// profileConfigModel/Effort look up persistent per-profile overrides
	// (agent.subagent_models / subagent_efforts).
	profileConfigModel  func(profile string) string
	profileConfigEffort func(profile string) string
	// mutationObserver is shared with spawned sub-agents for checkpoint capture.
	mutationObserver *checkpoint.MutationObserver
	writeRoots       *sandbox.WritableRootSet
	imageResolver    ImageRequestResolver
	hooksForSession  func(string) ToolHooks
	// capabilityRuntime is the session-shared MCP Host/specs substrate. Each
	// sub-agent gets its own use_capability frontend so ledger state stays
	// isolated while connections reuse the parent Host.
	capabilityRuntime *MCPCapabilityRuntime
}

// WithTranscripts enables persisted sub-agent results. The base model/effort
// are the parent provider identity used when no
// subagent override is configured.
func (t *SubagentRunner) WithTranscripts(store *SubagentStore, workspaceRoot, baseModel, baseEffort string) *SubagentRunner {
	t.transcripts = store
	t.workspaceRoot = strings.TrimSpace(workspaceRoot)
	t.baseModel = strings.TrimSpace(baseModel)
	t.baseEffort = strings.TrimSpace(baseEffort)
	return t
}

func (t *SubagentRunner) WithTranscriptIdentityResolver(resolve func(modelRef, effort string) (string, string)) *SubagentRunner {
	t.identityProfile = resolve
	return t
}

// WithAblation propagates the parent's benchmark arm so a sub-agent runs with
// the same subsystems switched off.
func (t *SubagentRunner) WithAblation(set ablation.Set) *SubagentRunner {
	t.ablation = set
	return t
}

// WithWorkspaceLease shares the parent's workspace-wide delivery write lease
// with every spawned sub-agent. A shared owner is required: independent owners
// in one session would deadlock when a child tries to write while its parent
// already retains the lease.
func (t *SubagentRunner) WithWorkspaceLease(owner *workspacelease.Owner) *SubagentRunner {
	t.workspaceLease = owner
	return t
}

// WithScheduler attaches the session-scoped concurrency and write-claim
// controller shared by batch and profile skill runners.
func (t *SubagentRunner) WithScheduler(s *SubagentScheduler) *SubagentRunner {
	t.scheduler = s
	return t
}

// Scheduler returns the attached session scheduler (may be nil in unit tests).
func (t *SubagentRunner) Scheduler() *SubagentScheduler {
	if t == nil {
		return nil
	}
	return t.scheduler
}

// WithProfileLookup enables profile resolution from the Skill store.
func (t *SubagentRunner) WithProfileLookup(lookup ProfileLookup) *SubagentRunner {
	t.profileLookup = lookup
	return t
}

// WithProfileConfigResolvers supplies persistent per-profile model/effort
// overrides (agent.subagent_models / subagent_efforts).
func (t *SubagentRunner) WithProfileConfigResolvers(model, effort func(profile string) string) *SubagentRunner {
	t.profileConfigModel = model
	t.profileConfigEffort = effort
	return t
}

// WithCapabilityRuntime attaches the session-shared MCP runtime so ordinary and
// read-only sub-agents receive a stable use_capability frontend without
// inheriting dynamic mcp__* schemas.
func (t *SubagentRunner) WithCapabilityRuntime(rt *MCPCapabilityRuntime) *SubagentRunner {
	if t != nil {
		t.capabilityRuntime = rt
	}
	return t
}

// buildTaskSpec resolves profile, tools, model/effort, and write claims for a
// single child. forceReadOnly forces the read-only registry.
func (t *SubagentRunner) buildTaskSpec(ctx context.Context, prompt, description, profile string, writePaths, tools []string, maxSteps int, model, effort, continueFrom string, background, forceReadOnly bool) (ProfileExecSpec, error) {
	spec := ProfileExecSpec{
		Task:    TaskSpec{Objective: prompt, Description: description},
		Worker:  WorkerSpec{Kind: "task", Name: "task", SystemPrompt: t.sysPrompt},
		Grant:   CapabilityGrant{CallTools: tools},
		Context: ContextRequest{ContinueFrom: strings.TrimSpace(continueFrom)},
		Sched:   SchedulerPolicy{MaxSteps: maxSteps, RunInBackground: background, Nested: SubagentDepth(ctx) > 0},
	}
	profile = strings.TrimSpace(profile)
	readOnly := forceReadOnly || PlanModeFromContext(ctx)
	var profileTools []string
	var profileModel, profileEffort string
	if profile != "" {
		def, err := ResolveProfileDefinition(t.profileLookup, profile)
		if err != nil {
			return ProfileExecSpec{}, err
		}
		spec.Worker.Profile = def.Name
		spec.Worker.Name = def.Name
		spec.Worker.Kind = "skill"
		spec.Worker.SystemPrompt = def.Body
		spec.Worker.UseProfilePrompt = true
		profileTools = def.AllowedTools
		profileModel, profileEffort = def.Model, def.Effort
		if def.ReadOnly {
			readOnly = true
		}
	}
	spec.Grant.ReadOnly = readOnly
	spec.Grant.ProfileTools = profileTools

	configModel, configEffort := "", ""
	if profile != "" {
		if t.profileConfigModel != nil {
			configModel = t.profileConfigModel(profile)
		}
		if t.profileConfigEffort != nil {
			configEffort = t.profileConfigEffort(profile)
		}
	}
	defaultModel, defaultEffort := t.subagentModel, t.subagentEffort
	if profile != "" && t.hasProfileDefaults {
		defaultModel, defaultEffort = t.profileDefaultModel, t.profileDefaultEffort
	}
	spec.Worker.Model, spec.Worker.Effort = ResolveModelEffort(
		configModel, configEffort,
		model, effort,
		profileModel, profileEffort,
		defaultModel, defaultEffort,
	)

	if len(writePaths) > 0 {
		return ProfileExecSpec{}, fmt.Errorf("write_paths is retired; writable subagents run serially")
	}
	if !readOnly {
		spec.Grant.WritePaths = WritePathSet{WholeWorkspace: true, WorkspaceRoot: t.workspaceRoot}
	}

	return spec, nil
}

// RunProfileSpec executes a unified child specification. Shared by batches,
// profiles, and previews so prompt, tools, permissions, and
// scheduling cannot drift across entry points.
func (t *SubagentRunner) RunProfileSpec(ctx context.Context, spec ProfileExecSpec) (result string, err error) {
	if t == nil {
		return "", fmt.Errorf("subagent runner is not configured")
	}
	if spec.Sched.RunInBackground || spec.Context.ContinueFrom != "" {
		return "", fmt.Errorf("background and continuation are retired; use spawn_agents")
	}
	spec.Grant.ReadOnly = spec.Grant.ReadOnly || PlanModeFromContext(ctx)
	// One progress owner publishes each child terminal after resource cleanup.
	trk := newSubagentProgressTracker(ctx, subSink(ctx))
	parentCtx := ctx

	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("subagent panic: %v", p)
		}
		trk.finish(parentCtx.Err(), err)
	}()
	trk.queued()
	if strings.TrimSpace(spec.Task.Objective) == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if strings.TrimSpace(spec.Worker.SystemPrompt) == "" {
		if spec.Worker.UseProfilePrompt {
			return "", fmt.Errorf("profile system prompt is empty")
		}
		spec.Worker.SystemPrompt = t.sysPrompt
	}

	ctx, maxSteps := t.childMaxStepsForSpec(ctx, &spec)
	childDepth, err := t.nextSubagentDepth(ctx)
	if err != nil {
		return "", err
	}

	toolNames, err := IntersectToolLists(t.parentReg, spec.Grant.ProfileTools, spec.Grant.CallTools)
	if err != nil {
		return "", err
	}
	subReg, childWriteRoots, err := t.buildSubagentRegistry(spec, toolNames, childDepth)
	if err != nil {
		return "", err
	}

	modelRef, effortRef := spec.Worker.Model, spec.Worker.Effort
	usageModelRef := t.usageModelRef(modelRef, effortRef)
	parentID, parentSink, _, _ := CallContext(ctx)
	run, err := t.prepareTranscriptRunWithPrompt(ctx, subReg, modelRef, effortRef, spec.Context.parentSession(ctx), parentID, spec.Context.ContinueFrom, spec.Worker.SystemPrompt, spec.Worker.Kind, spec.Worker.Name)
	if err != nil {
		return "", err
	}
	ctx, cancelRun := run.CancellableContext(ctx)
	releaseSlot := func() {}
	var answer string
	defer func() {
		defer releaseSlot()
		defer cancelRun()
		if problem := recover(); problem != nil {
			err = fmt.Errorf("subagent panic: %v", problem)
		}
		result, err = t.finishSubagentRun(run, answer, err)
		phase, outcome := terminalSubagentLifecycle(err)
		if err == nil {
			outcome.FinalAnswer = answer
		}
		emitSubagentLifecycle(parentSink, phase, parentID, spec.Worker.Name, usageModelRef, effortRef, run, outcome)
	}()
	prov, pricing, ctxWin, err := t.resolveSubSessionRuntime(modelRef, effortRef)
	if err != nil {
		return "", fmt.Errorf("sub-agent profile: %w", err)
	}
	emitSubagentLifecycle(parentSink, "child_created", parentID, spec.Worker.Name, usageModelRef, effortRef, run, nil)

	isWriter := t.writableRun(spec)
	acquireReq := AcquireRequest{
		Writer:     isWriter,
		WritePaths: spec.Grant.WritePaths,
		Nested:     spec.Sched.Nested,
		Label:      firstNonEmpty(spec.Task.Description, spec.Worker.Name, "task"),
	}
	recoveryTaskID := subagentRecoveryTaskID(ctx, run.Ref)
	var mutationObserver *checkpoint.MutationObserver
	if t.mutationObserver != nil {
		turn := t.mutationObserver.OwnershipTurn()
		mutationObserver = t.mutationObserver.CloneForSubagent(recoveryTaskID, turn, false)
	}
	runSession := func(runCtx context.Context, sink event.Sink) (answer string, runErr error) {
		defer func() {
			if problem := recover(); problem != nil {
				runErr = fmt.Errorf("subagent panic: %v", problem)
			}
		}()
		if spec.Grant.ReadOnly {
			return t.runReadOnlySubSession(runCtx, composeChildTaskPrompt(spec), subReg, sink, maxSteps, prov, pricing, ctxWin, run, childDepth, recoveryTaskID, usageModelRef, mutationObserver)
		}
		return t.runSubSession(runCtx, composeChildTaskPrompt(spec), subReg, sink, maxSteps, prov, pricing, ctxWin, run, childDepth, recoveryTaskID, usageModelRef, mutationObserver, childWriteRoots)
	}

	// Foreground: acquire a slot (queue if needed), then run synchronously.
	if spec.Sched.afterWriter != nil {
		select {
		case <-spec.Sched.afterWriter:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	var claimID int64
	releaseSlot, claimID, err = t.acquireSlot(ctx, acquireReq)
	if err != nil {
		return "", err
	}
	ctx = WithSubagentClaimID(ctx, claimID)
	trk.running()
	emitSubagentLifecycle(parentSink, "child_running", parentID, spec.Worker.Name, usageModelRef, effortRef, run, nil)
	answer, err = runSession(ctx, trk.wrap())
	return answer, err
}

func (t *SubagentRunner) acquireSlot(ctx context.Context, req AcquireRequest) (func(), int64, error) {
	noop := func() {}
	if t.scheduler == nil {
		return noop, 0, nil
	}
	return t.scheduler.AcquireWithID(ctx, req)
}

func (t *SubagentRunner) prepareTranscriptRunWithPrompt(ctx context.Context, subReg *tool.Registry, modelRef, effortRef, parentSession, parentID, continueFrom, systemPrompt, kind, name string) (*SubagentRun, error) {
	continueFrom = strings.TrimSpace(continueFrom)
	parentSession = strings.TrimSpace(parentSession)

	if systemPrompt == "" {
		systemPrompt = t.sysPrompt
	}
	if kind == "" {
		kind = "task"
	}
	if name == "" {
		name = "task"
	}
	if parentSession == "" {
		if continueFrom != "" {
			return nil, fmt.Errorf("subagent continuation requires a persisted session; none is active in this run")
		}
		return EphemeralSubagentRun(systemPrompt), nil
	}
	if t.transcripts == nil {
		return nil, fmt.Errorf("subagent transcript store is required")
	}
	identityModel, identityEffort := t.effectiveIdentity(modelRef, effortRef)
	spec := SubagentSpec{
		Kind:             kind,
		Name:             name,
		WorkspaceRoot:    t.workspaceRoot,
		ParentSession:    parentSession,
		ParentToolCallID: parentID,
		SystemPrompt:     systemPrompt,
		Registry:         subReg,
		ToolContext:      childToolIdentityContext(ctx),
		Model:            identityModel,
		Effort:           identityEffort,
		ResumedFrom:      continueFrom,
	}
	if continueFrom != "" {
		return nil, fmt.Errorf("continue_from is retired; start a fresh subagent")
	}

	return t.transcripts.PrepareFresh(spec)
}

func childToolIdentityContext(ctx context.Context) context.Context {
	ctx = tool.WithoutGoalLifecycle(ctx)
	ctx = memory.WithoutQueue(ctx)
	ctx = jobs.WithoutManager(ctx)
	return planmode.WithActive(ctx, PlanModeFromContext(ctx))
}

func (t *SubagentRunner) effectiveIdentity(modelRef, effort string) (string, string) {
	if t.identityProfile != nil {
		model, eff := t.identityProfile(modelRef, effort)
		return strings.TrimSpace(model), strings.TrimSpace(eff)
	}
	return t.effectiveModelIdentity(modelRef), t.effectiveEffortIdentity(effort)
}

// usageModelRef returns the canonical provider/model identity of the runtime
// selected for a child. The resolver expands aliases and supplies the parent
// model when no child override is configured.
func (t *SubagentRunner) usageModelRef(modelRef, effort string) string {
	model, _ := t.effectiveIdentity(modelRef, effort)
	if model != "" {
		return model
	}
	return firstNonEmpty(modelRef, t.baseModel, t.subagentModel)
}

func (t *SubagentRunner) effectiveModelIdentity(modelRef string) string {
	if strings.TrimSpace(modelRef) != "" {
		return strings.TrimSpace(modelRef)
	}
	return strings.TrimSpace(t.baseModel)
}

func (t *SubagentRunner) effectiveEffortIdentity(effort string) string {
	if strings.TrimSpace(effort) != "" {
		return strings.TrimSpace(effort)
	}
	return strings.TrimSpace(t.baseEffort)
}

// buildSubReg returns the sub-agent's tool set: the named whitelist (minus
// unavailable sub-agent tools), or every parent tool except those tools.
func (t *SubagentRunner) buildSubReg(names []string, childDepth int) *tool.Registry {
	return SubagentToolRegistryForDepthWithRuntime(t.parentReg, names, childDepth, t.maxDepth(), t.capabilityRuntime)
}

func (t *SubagentRunner) maxDepth() int { return 1 }

func (t *SubagentRunner) nextSubagentDepth(ctx context.Context) (int, error) {
	if SubagentDepth(ctx) > 0 {
		return 0, fmt.Errorf("subagents cannot delegate")
	}
	return 1, nil
}

// FilterRegistry builds a sub-registry from parent: the named whitelist (empty =
// every parent tool), minus any excluded names. Used to scope what a spawned
// sub-agent — a `task` sub-agent or a subagent skill — may call, e.g. excluding
// `task` to bar recursive nesting, or restricting to a skill's allowed-tools.
// Direct MCP tools may be copied here; callers that need a stable MCP surface
// should strip them and attach use_capability via attachSubagentCapabilityProxy.
func FilterRegistry(parent *tool.Registry, names []string, exclude ...string) *tool.Registry {
	sub := tool.NewRegistry()
	if parent == nil {
		return sub
	}
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	customAllowlist := len(names) > 0
	src := normalizeSubagentShellNames(parent, names)
	if !customAllowlist {
		src = parent.Names()
	} else {
		src = expandToolPatterns(parent, src)
	}
	for _, name := range src {
		if ex[name] {
			continue
		}
		// MCP never enters through the generic filter when named as capability
		// ids; model-visible mcp__* may still be listed for conversion later.
		if strings.HasPrefix(name, "mcp-tool:") || strings.HasPrefix(name, "mcp-server:") {
			continue
		}
		tl, ok := parent.Get(name)
		if !ok {
			continue
		}
		sub.Add(tl)
	}
	return sub
}

// stripDirectMCPTools removes provider-visible mcp__* tools so sub-agents use
// only the stable use_capability proxy for MCP.
func stripDirectMCPTools(reg *tool.Registry) {
	if reg == nil {
		return
	}
	for _, name := range append([]string(nil), reg.Names()...) {
		if strings.HasPrefix(name, tool.MCPNamePrefix) {
			reg.RemovePrefix(name)
		}
	}
}

// restrictedCapabilityProxy preserves a subagent allowed-tools boundary when
// MCP is available only through use_capability. The pseudo mcp-tool: and
// mcp-server: entries never become provider tools; they select one proxy schema
// whose resolver rejects every capability outside the exact allowlist.
//
// Provider-visible name/description/schema stay identical to the unrestricted
// proxy so allowlist expansion never changes the child cache prefix. Allowlist
// enforcement is host-local (check + filtered list results).
type restrictedCapabilityProxy struct {
	tool.Tool
	resolver tool.CallResolver
	allowed  map[string]bool
	// servers is the set of MCP server names implied by allowed IDs; list
	// results are filtered to this set so profile isolation covers discovery.
	servers map[string]bool
}

func (t *restrictedCapabilityProxy) ClassifyCall(args json.RawMessage) tool.CallClass {
	if t == nil || t.check(args) != nil {
		return tool.CallClass{}
	}
	classifier, ok := t.Tool.(tool.BatchClassifier)
	if !ok {
		return tool.CallClass{}
	}
	return classifier.ClassifyCall(args)
}

// Description is fixed: never embed dynamic capability IDs (they change with
// MCP install/tool-list and would break the stable provider tool prefix).
func (t *restrictedCapabilityProxy) Description() string {
	return t.Tool.Description()
}

func (t *restrictedCapabilityProxy) check(args json.RawMessage) error {
	var p struct {
		Action       string `json:"action"`
		CapabilityID string `json:"capability_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Errorf("invalid args: %w", err)
	}
	action := strings.ToLower(strings.TrimSpace(p.Action))
	if action == "list" || action == "search" {
		return nil
	}
	id := strings.TrimSpace(p.CapabilityID)
	if id == "" {
		return fmt.Errorf("capability_id is required")
	}
	if id == sessionToolResultCapabilityID {
		return nil
	}
	if !t.allowed[id] {
		return fmt.Errorf("capability %q is outside this subagent's allowed-tools", id)
	}
	return nil
}

func (t *restrictedCapabilityProxy) ResolveCall(ctx context.Context, args json.RawMessage) (tool.ResolvedCall, error) {
	if err := t.check(args); err != nil {
		return tool.ResolvedCall{}, err
	}
	rc, err := t.resolver.ResolveCall(ctx, args)
	if err != nil {
		return rc, err
	}
	var p struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(args, &p)
	action := strings.ToLower(strings.TrimSpace(p.Action))
	if rc.SkipExecute {
		switch action {
		case "list":
			rc.Result = filterCapabilityListResult(rc.Result, t.servers)
		case "search":
			rc.Result = filterCapabilitySearchResult(rc.Result, t.allowed)
		}
	}
	return rc, nil
}

func (t *restrictedCapabilityProxy) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := t.check(args); err != nil {
		return "", err
	}
	out, err := t.Tool.Execute(ctx, args)
	if err != nil {
		return out, err
	}
	var p struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(args, &p)
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "list":
		return filterCapabilityListResult(out, t.servers), nil
	case "search":
		return filterCapabilitySearchResult(out, t.allowed), nil
	}
	return out, nil
}

// validMCPServerCapabilityID accepts mcp-server:<non-empty-name> only.
func validMCPServerCapabilityID(id string) (server string, ok bool) {
	return tool.ParseMCPServerReference(id)
}

// validMCPToolCapabilityID accepts mcp-tool:<server>/<tool> with both parts non-empty.
func validMCPToolCapabilityID(id string) (server, raw string, ok bool) {
	return tool.ParseMCPToolReference(id)
}

func serversFromCapabilityAllowlist(allowed map[string]bool) map[string]bool {
	servers := map[string]bool{}
	for id := range allowed {
		id = strings.TrimSpace(id)
		if server, ok := validMCPServerCapabilityID(id); ok {
			servers[server] = true
			continue
		}
		if server, _, ok := validMCPToolCapabilityID(id); ok {
			servers[server] = true
		}
	}
	return servers
}

// attachSubagentCapabilityProxy installs a per-agent use_capability frontend.
// Any parent-copied proxy is replaced so children never share Executor ledger
// state. No allowlist → full proxy. Explicit allowlist with MCP names →
// restricted proxy. Explicit "use_capability" → full proxy. Explicit allowlist
// without MCP entries → no proxy.
func attachSubagentCapabilityProxy(parent, sub *tool.Registry, names []string, runtime *MCPCapabilityRuntime) {
	if sub == nil {
		return
	}
	// Drop any provider-copied use_capability so we always install an isolated
	// frontend (shared Host/runtime, independent ledger/audit).
	if _, ok := sub.Get("use_capability"); ok {
		sub.RemovePrefix("use_capability")
	}
	frontend := newSubagentCapabilityFrontend(parent, runtime)
	if frontend == nil {
		return
	}
	if len(names) == 0 || allowlistRequestsUnrestrictedProxy(names) {
		sub.Add(frontend)
		return
	}
	allowed := mcpCapabilityAllowlist(parent, names)
	if len(allowed) == 0 {
		// Custom allowlist with no valid MCP entries: do not expose the proxy.
		return
	}
	servers := serversFromCapabilityAllowlist(allowed)
	if len(servers) == 0 {
		// Incomplete capability IDs produced an empty server set: fail closed
		// rather than installing a restricted proxy that would list everything.
		return
	}
	resolver, ok := frontend.(tool.CallResolver)
	if !ok {
		return
	}
	sub.Add(&restrictedCapabilityProxy{
		Tool:     frontend,
		resolver: resolver,
		allowed:  allowed,
		servers:  servers,
	})
}

func newSubagentCapabilityFrontend(parent *tool.Registry, runtime *MCPCapabilityRuntime) tool.Tool {
	if runtime != nil {
		return runtime.NewFrontend(nil, nil)
	}
	if parent == nil {
		return nil
	}
	inner, ok := parent.Get("use_capability")
	if !ok {
		return nil
	}
	return cloneCapabilityFrontend(inner)
}

// mcpCapabilityAllowlist converts profile/call tool names into capability IDs
// for the restricted use_capability proxy. Accepts complete mcp-tool:<s>/<t>,
// mcp-server:<s>, model-visible mcp__* names, and wildcards expanded against
// the parent. Incomplete prefixes such as "mcp-server:" or "mcp-tool:foo" are
// rejected so they cannot install a restricted proxy with an empty server set.
func mcpCapabilityAllowlist(parent *tool.Registry, names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	expanded := names
	if parent != nil {
		expanded = expandToolPatterns(parent, names)
	}
	allowed := map[string]bool{}
	for _, name := range expanded {
		name = strings.TrimSpace(name)
		switch {
		case name == "use_capability":
			// Explicit proxy grant is handled as a full frontend by the caller
			// when this is the only MCP-related entry; leave empty here so a
			// bare use_capability allowlist entry still installs unrestricted.
			continue
		case strings.HasPrefix(name, "mcp-server:"):
			if server, ok := validMCPServerCapabilityID(name); ok {
				allowed["mcp-server:"+server] = true
			}
		case strings.HasPrefix(name, "mcp-tool:"):
			if server, raw, ok := validMCPToolCapabilityID(name); ok {
				allowed["mcp-tool:"+server+"/"+raw] = true
			}
		default:
			if parent != nil {
				if tl, ok := parent.Get(name); ok {
					if m, ok := tl.(tool.MCPMetadata); ok {
						server := strings.TrimSpace(m.MCPServerName())
						raw := strings.TrimSpace(m.MCPRawToolName())
						if server != "" && raw != "" {
							allowed["mcp-tool:"+server+"/"+raw] = true
							continue
						}
					}
				}
			}
			if server, raw, ok := tool.SplitMCPName(name); ok {
				allowed["mcp-tool:"+server+"/"+raw] = true
			}
		}
	}
	return allowed
}

func allowlistRequestsUnrestrictedProxy(names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) == "use_capability" {
			return true
		}
	}
	return false
}

// ReadOnlySubagentToolRegistry returns the tool set exposed to read-only
// sub-agents: read-only research tools plus a bash wrapper that enforces the
// permission-layer read-only command policy at execution time. Workflow/meta tools are
// excluded even when their Tool.ReadOnly contract is true.
func ReadOnlySubagentToolRegistry(parent *tool.Registry, names []string) *tool.Registry {
	return ReadOnlySubagentToolRegistryForDepth(parent, names, 1, 1)
}

// ReadOnlySubagentToolRegistryForDepth returns the tool set exposed to read-only
// subagents, with delegation tools excluded. Direct mcp__* schemas are never exposed; MCP goes only
// through use_capability. Dynamic execution still requires authorized server +
// readOnlyHint + non-destructive (enforced by ReadOnlyExecution), so strict
// agents share the stable proxy schema and connection reuse without permission
// relaxation.
//
// Custom profile/call allowlists remain authoritative and convert MCP names
// into a capability-id allowlist on a restricted proxy.
func ReadOnlySubagentToolRegistryForDepth(parent *tool.Registry, names []string, childDepth, maxDepth int) *tool.Registry {
	return ReadOnlySubagentToolRegistryForDepthWithRuntime(parent, names, childDepth, maxDepth, nil)
}

// ReadOnlySubagentToolRegistryForDepthWithRuntime is the read-only registry
// builder with an optional session MCP runtime for proxy injection.
func ReadOnlySubagentToolRegistryForDepthWithRuntime(parent *tool.Registry, names []string, childDepth, maxDepth int, runtime *MCPCapabilityRuntime) *tool.Registry {
	exclude := append([]string(nil), subagentAlwaysHiddenTools...)
	if childDepth > 0 {
		exclude = append(exclude, subagentRecursiveTools...)
	} else {
		exclude = append(exclude, "task", "run_skill", "explore", "research", "review", "security_review")
	}
	exclude = append(exclude, subagentJobTools...)
	exclude = append(exclude, plannerNonResearchTools...)
	exclude = append(exclude, readOnlySubagentWorkflowTools...)
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	sub := tool.NewRegistry()
	if parent == nil {
		return sub
	}
	src := normalizeSubagentShellNames(parent, names)
	if len(src) == 0 {
		src = parent.Names()
	} else {
		src = expandToolPatterns(parent, src)
	}
	for _, name := range src {
		if ex[name] {
			continue
		}
		if strings.HasPrefix(name, "mcp-tool:") || strings.HasPrefix(name, "mcp-server:") {
			continue
		}
		tl, ok := parent.Get(name)
		if !ok {
			continue
		}
		_, parentHasPwsh := parent.Get("pwsh")
		if name == "bash" && parentHasPwsh {
			continue
		}
		if name == "bash" || name == "pwsh" {
			sub.Add(readOnlyBash{inner: tl})
			continue
		}
		// Direct MCP never enters the strict registry — use_capability only.
		if isInstalledMCPTool(tl) || strings.HasPrefix(name, tool.MCPNamePrefix) {
			continue
		}
		if !tl.ReadOnly() {
			continue
		}
		sub.Add(tl)
	}
	attachSubagentCapabilityProxy(parent, sub, names, runtime)
	return sub
}

// expandToolPatterns resolves explicit wildcard allowlist entries from imported
// agent profiles against the current registry. Expansion is deterministic and
// session-local, so optional MCP tools only enter a child after connection.
func expandToolPatterns(parent *tool.Registry, names []string) []string {
	if parent == nil {
		return nil
	}
	available := parent.Names()
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if !strings.ContainsAny(name, "*?[") {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
			continue
		}
		for _, candidate := range available {
			matched, err := filepath.Match(name, candidate)
			if err == nil && matched && !seen[candidate] {
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}

// FilterReadOnlyRegistry builds a sub-registry containing only tools whose
// ReadOnly contract is true, minus explicit exclusions. MCP tools must
// additionally come from an authorized server and must not carry
// destructiveHint.
func FilterReadOnlyRegistry(parent *tool.Registry, exclude ...string) *tool.Registry {
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	sub := tool.NewRegistry()
	if parent == nil {
		return sub
	}
	for _, name := range parent.Names() {
		if ex[name] {
			continue
		}
		tl, ok := parent.Get(name)
		if !ok || !tl.ReadOnly() {
			continue
		}
		if isInstalledMCPTool(tl) && (!mcpServerAuthorized(tl) || mcpDestructiveHint(tl)) {
			continue
		}
		sub.Add(tl)
	}
	return sub
}

func (t *SubagentRunner) resolveSubSessionRuntime(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error) {
	prov, pricing, ctxWin := t.prov, t.pricing, t.contextWindow
	if t.resolveProvider != nil && (modelRef != "" || effort != "") {
		p, pr, cw, err := t.resolveProvider(modelRef, effort)
		if err != nil {
			return nil, nil, 0, err
		}
		prov, pricing, ctxWin = p, pr, cw
	}
	return prov, pricing, ctxWin, nil
}

func (t *SubagentRunner) runSubSession(ctx context.Context, prompt string, subReg *tool.Registry, sink event.Sink, maxSteps int, prov provider.Provider, pricing *provider.Pricing, ctxWin int, run *SubagentRun, childDepth int, recoveryTaskID, modelRef string, mutationObserver *checkpoint.MutationObserver, writeRoots *sandbox.WritableRootSet) (string, error) {
	opts := t.subagentOptions(ctx, maxSteps, pricing, ctxWin, childDepth, recoveryTaskID, mutationObserver)
	opts.SessionCheckpointer = run.Persistence
	if writeRoots != nil {
		opts.WriteRoots = writeRoots
	}
	opts.ModelRef = modelRef
	// Capture the pristine task before host framing is prepended: delivery
	// intent classification must judge the task, not the wrapper.
	opts.ClassifierTaskText = prompt
	prompt = t.withWorkspaceContext(prompt)
	// The child provider owns the final vision decision. Text-only providers
	// retain the attachment metadata but omit image parts during serialization.
	ctx = withSubagentTurnImages(ctx)
	return RunSubAgentWithSession(ctx, prov, subReg, run.Session, prompt, opts, sink)
}

func (t *SubagentRunner) runReadOnlySubSession(ctx context.Context, prompt string, subReg *tool.Registry, sink event.Sink, maxSteps int, prov provider.Provider, pricing *provider.Pricing, ctxWin int, run *SubagentRun, childDepth int, recoveryTaskID, modelRef string, mutationObserver *checkpoint.MutationObserver) (string, error) {
	opts := t.subagentOptions(ctx, maxSteps, pricing, ctxWin, childDepth, recoveryTaskID, mutationObserver)
	opts.SessionCheckpointer = run.Persistence
	opts.ModelRef = modelRef
	// Capture the pristine task before host framing is prepended: delivery
	// intent classification must judge the task, not the wrapper.
	opts.ClassifierTaskText = prompt
	prompt = t.withWorkspaceContext(prompt)
	ctx = withSubagentTurnImages(ctx)
	return RunReadOnlySubAgentWithSession(ctx, prov, subReg, run.Session, prompt, opts, sink)
}

func subagentRecoveryTaskID(ctx context.Context, ref string) string {
	if ref = strings.TrimSpace(ref); ref != "" {
		return "subagent:" + ref
	}
	if callID, _, _, ok := CallContext(ctx); ok && strings.TrimSpace(callID) != "" {
		return "subagent:" + strings.TrimSpace(callID)
	}
	return "subagent"
}

func (t *SubagentRunner) WithWriteRoots(set *sandbox.WritableRootSet) *SubagentRunner {
	if t == nil {
		return nil
	}
	t.writeRoots = set
	return t
}

// WithMutationObserver shares the host mutation observer with spawned sub-agents.
// Foreground children inherit the parent ownership turn; background children
// keep the turn that spawned them (set via OwnershipTurn at Begin).
func (t *SubagentRunner) WithMutationObserver(obs *checkpoint.MutationObserver) *SubagentRunner {
	if t == nil {
		return nil
	}
	t.mutationObserver = obs
	return t
}

func (t *SubagentRunner) withWorkspaceContext(prompt string) string {
	if t == nil {
		return prompt
	}
	ctx := subagentWorkspaceContext(t.workspaceRoot)
	if ctx == "" {
		return prompt
	}
	return ctx + "\n\n" + prompt
}

func subagentWorkspaceContext(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	// Wording note: avoid incidental action verbs ("resolve", "fix", …) in this
	// host framing — it is prepended to every sub-agent prompt and must never
	// read as task intent (see classifierTaskText, which also strips it).
	return `<workspace-context event="SubagentWorkspace">
Current workspace: ` + strconv.Quote(root) + `
File tools interpret relative paths against this workspace. For project inspection, prefer "." or relative paths unless the user explicitly named another absolute path.
</workspace-context>`
}

func FormatSubagentReference(run *SubagentRun) string {
	if run == nil || run.Ref == "" {
		return ""
	}
	return "Subagent reference: " + run.Ref + "\nRead the retained result with read_subagent_result. Start a fresh task for further work."
}

// GuardSubagentHostDecisionText appends a fixed boundary warning only when a
// child agent result appears to discuss host approval or user-owned decisions.
// The implementation lives in internal/tool so the skill tools share the exact
// same phrase list and notice.
func GuardSubagentHostDecisionText(answer string) string {
	return tool.GuardSubagentHostDecisionText(answer)
}

// RunSubAgentWithSession runs the shared runner's fresh child session and returns
// its final assistant answer.
//
// Each call installs an independent session-private temporary directory Manager
// so parent and sibling agents never share temporary files.
// Each run gets a fresh temporary directory, released only after execution exits.
func RunSubAgentWithSession(ctx context.Context, prov provider.Provider, reg *tool.Registry, sess *Session, prompt string, opts Options, sink event.Sink) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("sub-agent session is nil")
	}
	ctx = WithoutTurnContextBundle(ctx)
	// Isolate temporary files for this run before any tool execution.
	ctx = tool.WithoutGoalLifecycle(ctx)
	if opts.MemoryQueue != nil {
		ctx = memory.WithQueue(ctx, opts.MemoryQueue)
	} else {
		ctx = memory.WithoutQueue(ctx)
	}
	if opts.Jobs == nil {
		ctx = jobs.WithoutManager(ctx)
	}
	ctx, releaseTemp := withSubagentSessionTemp(ctx)
	defer releaseTemp()
	opts.SessionTemp = sessiontemp.FromContext(ctx)
	if opts.SubagentDepth > 0 {
		ctx = WithSubagentDepth(ctx, opts.SubagentDepth)
	}
	// Callers that wrap the prompt themselves (runSubSession) set
	// ClassifierTaskText before wrapping; for everyone else the prompt is
	// still pristine here, so capture it before host framing is prepended.
	if strings.TrimSpace(opts.ClassifierTaskText) == "" {
		opts.ClassifierTaskText = prompt
	}
	planWorkflow := PlanModeFromContext(ctx)
	if opts.SubagentDepth > 0 && isFreshSubagentSession(sess) {
		prompt = subagentStartContext + "\n\n" + prompt
	}
	if planWorkflow && !strings.Contains(prompt, planmode.Marker) {
		prompt = planmode.Marker + "\n\n" + prompt
	}
	// Nested reasoning stays isolated; the parent consumes only final Content.
	// Require it so a reasoning-only stop cannot fall back to older tool text.
	opts.RequireVisibleFinal = true
	sub := New(prov, reg, sess, opts, sink)
	sub.SetPlanMode(planWorkflow)
	if err := sub.Run(ctx, prompt); err != nil {
		// Preserve actual partial child execution even when the child fails.
		mergeChildEvidence(ctx, sub)
		return "", fmt.Errorf("sub-agent: %w", err)
	}
	mergeChildEvidence(ctx, sub)
	if answer := latestAssistantAnswer(sess); answer != "" {
		return composeSubagentAnswer(ctx, answer, sub, opts.ClassifierTaskText), nil
	}
	return "", fmt.Errorf("sub-agent finished without producing a final answer")
}

// readOnlyAgentConstruction is the single pairing every strictly read-only
// loop shares: the permanent ReadOnlyExecution flag plus the final registry
// filter. RunReadOnlySubAgentWithSession applies both before constructing a
// child. The interactive two-model planner uses NewPlannerAgent instead
// (PlannerMCPExecution).
func readOnlyAgentConstruction(reg *tool.Registry, opts Options) (*tool.Registry, Options) {
	opts.ReadOnlyExecution = true
	opts.PlannerMCPExecution = false
	return strictReadOnlyExecutionRegistry(reg), opts
}

// NewPlannerAgent constructs the interactive two-model planner: permanent
// ReadOnlyExecution still blocks bash, file writers, and ordinary non-MCP
// writers, while PlannerMCPExecution allows authorized, non-destructive MCP
// through the stable use_capability proxy without requiring readOnlyHint.
func NewPlannerAgent(prov provider.Provider, reg *tool.Registry, sess *Session, opts Options, sink event.Sink) *Agent {
	opts.ReadOnlyExecution = true
	opts.PlannerMCPExecution = true
	// The coordinator needs visible plan text to hand off to the executor;
	// reasoning shown in a frontend is not a substitute for that contract.
	opts.RequireVisibleFinal = true
	// Keep construction-time filter for ordinary tools; use_capability stays
	// because it is ReadOnly. Direct mcp__* tools are already excluded by
	// PlannerToolRegistry. Dynamic MCP targets are re-checked after resolve.
	reg = plannerExecutionRegistry(reg)
	return New(prov, reg, sess, opts, sink)
}

// plannerExecutionRegistry is the construction-time filter for NewPlannerAgent.
// It removes ordinary writers and destructive direct MCP tools while keeping
// use_capability and built-in research tools. Host-starting deferred MCP
// targets are allowed at execution time under PlannerMCPExecution.
func plannerExecutionRegistry(reg *tool.Registry) *tool.Registry {
	filtered := tool.NewRegistry()
	if reg == nil {
		return filtered
	}
	for _, name := range reg.Names() {
		target, ok := reg.Get(name)
		if !ok {
			continue
		}
		if name == "use_capability" {
			filtered.Add(target)
			continue
		}
		if strings.HasPrefix(name, tool.MCPNamePrefix) {
			// Defense in depth: planner never exposes direct MCP schemas.
			continue
		}
		if !target.ReadOnly() || mcpDestructiveHint(target) {
			continue
		}
		if h, ok := target.(tool.ReadOnlyExecutionHostMutation); ok && h.ReadOnlyExecutionHostMutation() {
			// Ordinary host mutations stay out; MCP startup is only via proxy.
			continue
		}
		filtered.Add(target)
	}
	return filtered
}

// RunReadOnlySubAgentWithSession is the construction boundary for every
// strictly read-only child loop. Registry filtering limits the visible surface;
// this permanent execution flag also re-checks targets resolved dynamically by
// proxy tools such as use_capability. It never enables PlannerMCPExecution.
func RunReadOnlySubAgentWithSession(ctx context.Context, prov provider.Provider, reg *tool.Registry, sess *Session, prompt string, opts Options, sink event.Sink) (string, error) {
	reg, opts = readOnlyAgentConstruction(reg, opts)
	return RunSubAgentWithSession(ctx, prov, reg, sess, prompt, opts, sink)
}

// strictReadOnlyExecutionRegistry is the final construction-time filter shared
// by every strict child. Callers still apply role-specific filtering (review,
// planner, profile allowlists), while this layer guarantees that a missed call
// site cannot expose writers, destructive MCP tools, readers from unauthorized
// servers, or an unauthorized host-starting target to the model.
func strictReadOnlyExecutionRegistry(reg *tool.Registry) *tool.Registry {
	filtered := tool.NewRegistry()
	if reg == nil {
		return filtered
	}
	for _, name := range reg.Names() {
		target, ok := reg.Get(name)
		if !ok || !target.ReadOnly() || mcpDestructiveHint(target) {
			continue
		}
		if isInstalledMCPTool(target) && !mcpServerAuthorized(target) {
			continue
		}
		if mutation, ok := target.(tool.ReadOnlyExecutionHostMutation); ok && mutation.ReadOnlyExecutionHostMutation() && !readOnlyExecutionAllowsMCPStartup(target) {
			continue
		}
		filtered.Add(target)
	}
	return filtered
}

// latestAssistantAnswer walks the session backwards for the last assistant
// message with content — that's the sub-agent's final answer. Intermediate
// assistant messages with tool_calls but no text don't count.
func latestAssistantAnswer(sess *Session) string {
	if sess == nil {
		return ""
	}
	for _, v := range slices.Backward(sess.Messages) {
		m := v
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content
		}
	}
	return ""
}

// mergeChildEvidence folds a sub-agent's real receipts into the parent ledger
// carried on ctx. Meta tools themselves are never mutations.
func mergeChildEvidence(ctx context.Context, sub *Agent) {
	if sub == nil {
		return
	}
	parent, ok := evidence.FromContext(ctx)
	if !ok || parent == nil {
		return
	}
	parent.MergeChild(sub.EvidenceSummary())
}

// EvidenceSummary exports this agent's turn-scoped receipts for parent merge.
func (a *Agent) EvidenceSummary() evidence.ChildEvidenceSummary {
	if a == nil || a.task.ledger == nil {
		return evidence.ChildEvidenceSummary{}
	}
	return a.task.ledger.Summary()
}

func isFreshSubagentSession(sess *Session) bool {
	if sess == nil {
		return false
	}
	snap := sess.Snapshot()
	return len(snap) == 1 && snap[0].Role == provider.RoleSystem
}

// NestedSink returns a sink that forwards a sub-agent's tool activity to the
// parent stream, nested under the tool call carried by ctx, so a frontend shows
// it beneath that call (the same nesting `task` uses). Falls back to the given
// sink when ctx carries no call context. Used by subagent skills.
func NestedSink(ctx context.Context, fallback event.Sink) event.Sink {
	parentID, parent, _, ok := CallContext(ctx)
	if !ok || parent == nil {
		return fallback
	}
	return subSinkFor(parentID, parent)
}

// subSink forwards a sub-agent's tool dispatch/result/progress events and
// billable usage to the parent's event stream. Only tool activity is nested
// visually; the sub-agent's text/reasoning stays isolated (progress previews
// travel as reserved ToolProgress channels, not as parent Text/Reasoning) and
// only its final answer is returned.
//
// The sub-agent's own turn/text/reasoning events are dropped — forwarding them
// would make the parent transcript noisy and could imply they belong to the
// parent model context, which they do not.
//
// Usage events are observability only, so forwarding them preserves billing
// totals without polluting the parent provider-visible prefix.
//
// Tool events are tagged with the parent task call's ID so a frontend nests them
// under it. The forwarded call IDs are namespaced with the parent ID so a
// sub-agent call can never collide with a parent call in the frontend's
// dispatch→result matching. ToolProgress covers both the sub-agent's real tool
// output and nested sub-agent progress previews, which ride the same sink so
// their IDs match the cards they belong to. Falls back to Discard when there's
// no parent stream (the headless run loop, or a direct Execute in tests).
func subSink(ctx context.Context) event.Sink {
	parentID, parent, _, ok := CallContext(ctx)
	if !ok || parent == nil {
		return event.Discard
	}
	return subSinkFor(parentID, parent)
}

// subSinkFor builds the nesting sink from an already-captured parent ID + stream,
// for the background path where the job runs under a context that no longer
// carries the call context. Falls back to Discard when there's no parent stream.
func subSinkFor(parentID string, parent event.Sink) event.Sink {
	if parent == nil {
		return event.Discard
	}
	return nestedSink{AuditForwarder: event.AuditForwarder{Inner: parent}, parentID: parentID, parent: parent}
}

func (t *SubagentRunner) WithProfileDefaults(model, effort string) *SubagentRunner {
	t.profileDefaultModel, t.profileDefaultEffort, t.hasProfileDefaults = model, effort, true
	return t
}
