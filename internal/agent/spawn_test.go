package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/tool"
)

type spawnProvider struct {
	mu     sync.Mutex
	calls  int
	run    func(context.Context, provider.Request) error
	answer string
}

type spawnLifecycleSink struct {
	event.Sink
	lifecycle chan event.SubagentLifecycleInfo
}

func (s *spawnLifecycleSink) RecordSubagentLifecycle(info event.SubagentLifecycleInfo) {
	s.lifecycle <- info
}

func TestQueuedSubagentCancellationPersistsAndPublishesTerminal(t *testing.T) {
	p := &spawnProvider{}
	s := spawnFixture(t, p)
	release, err := s.runner.scheduler.ReserveParentWrite(WritePathSet{WholeWorkspace: true})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	sink := &spawnLifecycleSink{Sink: event.Discard, lifecycle: make(chan event.SubagentLifecycleInfo, 8)}
	ctx, cancel := context.WithCancel(withCallContext(testTaskContext(), "queued", sink, nil, false))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.runner.RunProfileSpec(ctx, ProfileExecSpec{Task: TaskSpec{Objective: "write"}})
		done <- err
	}()
	var created event.SubagentLifecycleInfo
	select {
	case created = <-sink.lifecycle:
		if created.Phase != "child_created" {
			t.Fatalf("first event = %+v", created)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued child was not registered")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled child succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued cancellation did not settle")
	}
	select {
	case terminal := <-sink.lifecycle:
		if terminal.Phase != "child_cancelled" || terminal.Ref != created.Ref {
			t.Fatalf("terminal = %+v", terminal)
		}
	default:
		t.Fatal("missing queued cancellation lifecycle event")
	}
	meta, err := s.runner.transcripts.LoadMeta(created.Ref)
	if err != nil || meta.Status != SubagentInterrupted || p.calls != 0 {
		t.Fatalf("queued result: meta=%+v calls=%d err=%v", meta, p.calls, err)
	}
}

func TestSpawnProfileInvalidToolsRejectWholeBatch(t *testing.T) {
	for _, names := range [][]string{{" "}, {"read_file", "misspelled_tool"}, {"spawn_agents"}} {
		t.Run(fmt.Sprint(names), func(t *testing.T) {
			p := &spawnProvider{}
			s := spawnFixture(t, p)
			s.runner.WithProfileLookup(func(name string) (ProfileDefinition, bool) {
				return ProfileDefinition{Name: name, Body: "Inspect code", AllowedTools: names}, true
			})
			_, err := s.Execute(testTaskContext(), []byte(`{"tasks":[{"title":"valid","task":"inspect"},{"title":"invalid","task":"inspect","profile":"broken"}]}`))
			if err == nil || p.calls != 0 {
				t.Fatalf("invalid profile launched work: calls=%d err=%v", p.calls, err)
			}
		})
	}
}

func (*spawnProvider) Name() string { return "spawn-test" }
func (p *spawnProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if p.run != nil {
		if err := p.run(ctx, req); err != nil {
			return nil, err
		}
	}
	ch := make(chan provider.Chunk, 2)
	answer := p.answer
	if answer == "" {
		answer = strings.Repeat("中文回答", 1000)
	}
	ch <- provider.Chunk{Type: provider.ChunkText, Text: answer}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func TestSpawnFailureDoesNotCancelIndependentSibling(t *testing.T) {
	p := &spawnProvider{run: func(_ context.Context, req provider.Request) error {
		if strings.Contains(lastUser(req), "broken child") {
			panic("fixture failure")
		}
		return nil
	}}
	s := spawnFixture(t, p)
	out, err := s.Execute(testTaskContext(), []byte(`{"tasks":[{"title":"broken","task":"broken child","read_only":true},{"title":"healthy","task":"inspect","read_only":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Agents []spawnResult `json:"agents"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Agents) != 2 || result.Agents[0].Status == SubagentOutcomeCompleted || result.Agents[0].Ref == "" || result.Agents[1].Status != SubagentOutcomeCompleted {
		t.Fatal(out)
	}
	meta, err := s.runner.transcripts.LoadMeta(result.Agents[0].Ref)
	if err != nil || meta.Status != SubagentFailed {
		t.Fatalf("panic outcome not persisted: %+v %v", meta, err)
	}
}

func TestSpawnEscapedAnswersRemainValidBoundedJSON(t *testing.T) {
	s := spawnFixture(t, &spawnProvider{answer: strings.Repeat("\x00", 4000)})
	tasks := make([]map[string]any, 8)
	for i := range tasks {
		tasks[i] = map[string]any{"title": fmt.Sprint(i), "task": "inspect", "read_only": true}
	}
	args, _ := json.Marshal(map[string]any{"tasks": tasks})
	out, err := s.Execute(testTaskContext(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(out)) || len(out) > maxToolOutputBytes {
		t.Fatalf("invalid/big result: bytes=%d", len(out))
	}
	var result struct {
		Agents []spawnResult `json:"agents"`
	}
	_ = json.Unmarshal([]byte(out), &result)
	for _, child := range result.Agents {
		if child.Ref == "" || !child.More {
			t.Fatalf("lost retrieval metadata: %+v", child)
		}
	}
}

func TestNamedProfileDefaultsDoNotInheritTaskSpecificModel(t *testing.T) {
	s := spawnFixture(t, &spawnProvider{})
	s.runner.subagentModel = "task-only"
	s.runner.WithProfileDefaults("profile-default", "high")
	s.runner.profileLookup = func(name string) (ProfileDefinition, bool) {
		return ProfileDefinition{Name: name, Body: "inspect"}, true
	}
	_, specs, err := s.prepareTasks(testTaskContext(), []byte(`{"tasks":[{"title":"ordinary","task":"inspect"},{"title":"named","task":"inspect","profile":"specialist"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if specs[0].Worker.Model != "task-only" || specs[1].Worker.Model != "profile-default" {
		t.Fatalf("wrong defaults: %+v", specs)
	}
}
func spawnFixture(t *testing.T, p *spawnProvider) *Spawner {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	reg.Add(fakeTool{name: "write_file"})
	runner := newTestSubagentRunner(t, p, reg, "child system", "", "", nil).WithScheduler(NewSubagentScheduler(6, 1))
	return NewSpawner(runner)
}
func receiveSpawn(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("subagent did not reach the expected boundary")
		return ""
	}
}

func TestSpawnPreflightStartsNothingForInvalidBatch(t *testing.T) {
	for _, args := range []string{
		`{"tasks":[]}`,
		`{"tasks":[{"title":"","task":"x"}]}`,
		`{"tasks":[{"title":"ok","task":"x"},{"title":"bad","task":"x","tools":[]}]}`,
		`{"tasks":[{"title":"ok","task":"x"},{"title":"bad","task":"x","profile":"missing"}]}`,
		`{"tasks":[{"title":"bad","task":"x","tools":["missing"]}]}`,
		`{"tasks":[{"title":"bad","task":"x","continue_from":"sa_old"}]}`,
		`{"tasks":[{"title":"bad","task":"x","tools":null}]}`,
	} {
		p := &spawnProvider{}
		s := spawnFixture(t, p)
		if _, err := s.Execute(testTaskContext(), json.RawMessage(args)); err == nil {
			t.Fatalf("accepted %s", args)
		}
		if p.calls != 0 {
			t.Fatal("invalid batch started a provider")
		}
	}
}

func TestSpawnBatchBoundsAndArchivedUnicodeResults(t *testing.T) {
	for _, n := range []int{1, 8, 9} {
		p := &spawnProvider{}
		s := spawnFixture(t, p)
		tasks := make([]map[string]any, n)
		for i := range tasks {
			tasks[i] = map[string]any{"title": fmt.Sprint(i), "task": "inspect", "read_only": true}
		}
		args, _ := json.Marshal(map[string]any{"tasks": tasks})
		out, err := s.Execute(testTaskContext(), args)
		if n == 9 {
			if err == nil || p.calls != 0 {
				t.Fatal("oversized batch executed")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Agents []spawnResult `json:"agents"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Agents) != n {
			t.Fatal(out)
		}
		for i, item := range result.Agents {
			if item.Title != fmt.Sprint(i) || item.Status != SubagentOutcomeCompleted || item.Ref == "" || !item.More {
				t.Fatalf("invalid result %+v", item)
			}
			full, status, err := s.runner.transcripts.ReadFinalAnswer(item.Ref, "parent-session", s.runner.workspaceRoot)
			if err != nil || status != SubagentCompleted || full != strings.Repeat("中文回答", 1000) {
				t.Fatalf("archive: %s %v", status, err)
			}
		}
	}
}

func TestSpawnSerialWritersDoNotBlockParallelReader(t *testing.T) {
	started := make(chan string, 3)
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	writers := 0
	p := &spawnProvider{run: func(ctx context.Context, req provider.Request) error {
		text := lastUser(req)
		name := "reader"
		writer := !strings.Contains(text, "reader")
		if strings.Contains(text, "first") {
			name = "first"
		} else if writer {
			name = "second"
		}
		if writer {
			mu.Lock()
			writers++
			if writers != 1 {
				t.Error("writable children overlapped")
			}
			mu.Unlock()
			defer func() { mu.Lock(); writers--; mu.Unlock() }()
		}
		started <- name
		if name == "first" {
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}}
	s := spawnFixture(t, p)
	done := make(chan error, 1)
	go func() {
		_, err := s.Execute(testTaskContext(), []byte(`{"tasks":[{"title":"first","task":"first"},{"title":"second","task":"second"},{"title":"reader","task":"reader","read_only":true}]}`))
		done <- err
	}()
	seen := map[string]bool{receiveSpawn(t, started): true}
	seen[receiveSpawn(t, started)] = true
	if !seen["first"] || !seen["reader"] {
		t.Fatalf("waiting writer starved reader: %v", seen)
	}
	close(releaseFirst)
	if got := receiveSpawn(t, started); got != "second" {
		t.Fatalf("next child = %s", got)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("batch did not finish")
	}
}

func TestCancelledQueuedWriterCannotLetSuccessorOvertakePredecessor(t *testing.T) {
	started := make(chan string, 3)
	p := &spawnProvider{run: func(_ context.Context, req provider.Request) error {
		started <- lastUser(req)
		return nil
	}}
	s := spawnFixture(t, p)
	s.runner.WithProfileLookup(func(name string) (ProfileDefinition, bool) {
		return ProfileDefinition{Name: name, Body: "perform the task", Model: name}, true
	})
	blocked := make(chan struct{})
	proceed := make(chan struct{})
	defer close(proceed)
	var mu sync.Mutex
	firstResolutions := 0
	s.runner.resolveProvider = func(model, _ string) (provider.Provider, *provider.Pricing, int, error) {
		if model == "first" {
			mu.Lock()
			firstResolutions++
			n := firstResolutions
			mu.Unlock()
			if n == 2 { // preflight succeeds; actual child setup is held back
				close(blocked)
				<-proceed
			}
		}
		return p, nil, 0, nil
	}
	sink := &spawnLifecycleSink{Sink: event.Discard, lifecycle: make(chan event.SubagentLifecycleInfo, 16)}
	ctx, cancelBatch := context.WithCancel(withCallContext(testTaskContext(), "batch", sink, nil, false))
	defer cancelBatch()
	done := make(chan error, 1)
	go func() {
		_, err := s.Execute(ctx, []byte(`{"tasks":[{"title":"first","task":"first writer","profile":"first"},{"title":"middle","task":"middle writer","profile":"middle"},{"title":"last","task":"last writer","profile":"last"}]}`))
		done <- err
	}()
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("first writer did not reach setup barrier")
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	cancelled := false
	for !cancelled {
		select {
		case info := <-sink.lifecycle:
			if info.Skill == "middle" && info.Phase == "child_created" {
				store := s.runner.transcripts.persistence.(*testSubagentPersistence)
				store.mu.Lock()
				cancel := store.records[info.Ref].cancel
				store.mu.Unlock()
				if cancel == nil {
					t.Fatal("queued child has no cancellation handle")
				}
				cancel()
			}
			cancelled = info.Skill == "middle" && info.Phase == "child_cancelled"
		case <-deadline.C:
			t.Fatal("middle writer did not cancel")
		}
	}
	select {
	case got := <-started:
		t.Fatalf("writer overtook blocked predecessor: %s", got)
	default:
	}
	proceed <- struct{}{}
	if got := receiveSpawn(t, started); !strings.Contains(got, "first writer") {
		t.Fatalf("first admitted writer = %s", got)
	}
	if got := receiveSpawn(t, started); !strings.Contains(got, "last writer") {
		t.Fatalf("last admitted writer = %s", got)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("batch did not finish")
	}
}

func TestSpawnCannotDelegateFromChild(t *testing.T) {
	p := &spawnProvider{}
	s := spawnFixture(t, p)
	if _, err := s.Execute(WithSubagentDepth(testTaskContext(), 1), []byte(`{"tasks":[{"title":"nested","task":"x"}]}`)); err == nil || p.calls != 0 {
		t.Fatal("nested delegation ran")
	}
}

func TestSpawnPublishesArchivedIdentityToDesktop(t *testing.T) {
	s := spawnFixture(t, &spawnProvider{})
	sink := &recordSink{}
	ctx := withCallContext(testTaskContext(), "batch", sink, nil, false)
	if _, err := s.Execute(ctx, []byte(`{"tasks":[{"title":"inspect","task":"inspect","read_only":true}]}`)); err != nil {
		t.Fatal(err)
	}
	results := sink.kinds(event.ToolResult)
	if len(results) != 1 || results[0].Tool.SubagentRef == "" || results[0].Tool.SubagentStatus != "completed" {
		t.Fatalf("desktop lost child identity or terminal state: %+v", results)
	}
}

func TestWriterSlotSurvivesCancellationUntilRunnerExits(t *testing.T) {
	started := make(chan string, 1)
	observed := make(chan struct{})
	exit := make(chan struct{})
	p := &spawnProvider{run: func(ctx context.Context, _ provider.Request) error {
		started <- "started"
		<-ctx.Done()
		close(observed)
		<-exit
		return ctx.Err()
	}}
	s := spawnFixture(t, p)
	ctx, cancel := context.WithCancel(testTaskContext())
	done := make(chan error, 1)
	go func() { _, err := s.Execute(ctx, []byte(`{"tasks":[{"title":"writer","task":"write"}]}`)); done <- err }()
	receiveSpawn(t, started)
	cancel()
	<-observed
	if release, err := s.runner.scheduler.ReserveParentWrite(WritePathSet{WholeWorkspace: true}); err == nil {
		release()
		t.Fatal("writer slot released before physical exit")
	}
	close(exit)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not settle")
	}
	release, err := s.runner.scheduler.ReserveParentWrite(WritePathSet{WholeWorkspace: true})
	if err != nil {
		t.Fatal(err)
	}
	release()
}
