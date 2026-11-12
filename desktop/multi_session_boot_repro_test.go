package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"deepzora/internal/config"
	"deepzora/internal/control"
)

func TestSameProjectBootedSessionsFinishWhileSiblingRuns(t *testing.T) {
	for _, longTool := range []bool{false, true} {
		t.Run(fmt.Sprintf("long_tool=%t", longTool), func(t *testing.T) {
			testSameProjectBootedSessionsFinishWhileSiblingRuns(t, longTool)
		})
	}
}

func testSameProjectBootedSessionsFinishWhileSiblingRuns(t *testing.T, longTool bool) {
	a := newManualSessionTestApp(t)
	a.metrics.Store(newMetricsAggregator(config.MemoryUserDir()))
	a.enableDeferredRebuildRetry()
	t.Cleanup(a.stopDeferredRebuildRetry)
	t.Cleanup(a.workspaceHub.close)
	root := t.TempDir()
	marker := filepath.Join(root, "tool-started")
	releaseFile := filepath.Join(root, "tool-release")
	started, release := make(chan struct{}), make(chan struct{})
	releaseProvider := sync.OnceFunc(func() {
		_ = os.WriteFile(releaseFile, []byte("release"), 0600)
		close(release)
	})
	var startedOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(body), "hold-first-session") {
			if longTool && !strings.Contains(string(body), `"tool_call_id"`) {
				toolName := "bash"
				command := "echo started > tool-started; while [ ! -f tool-release ]; do sleep 0.05; done"
				var request struct {
					Tools []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				_ = json.Unmarshal(body, &request)
				for _, advertised := range request.Tools {
					if advertised.Function.Name == "pwsh" {
						toolName = "pwsh"
						command = "Set-Content tool-started started; while (!(Test-Path tool-release)) { Start-Sleep -Milliseconds 50 }"
					}
				}
				args, _ := json.Marshal(map[string]any{"command": command, "description": "Hold the first session until the test releases it", "timeout_ms": 30000})
				payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "held-shell", "type": "function", "function": map[string]any{"name": toolName, "arguments": string(args)}}}}, "finish_reason": "tool_calls"}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
				return
			}
			startedOnce.Do(func() { close(started) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"I am the fixture assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	defer releaseProvider()
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "multi-session", Kind: "openai", BaseURL: server.URL, Models: []string{"test"}, NoProxy: true}, "fixture-key"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("multi-session/test"); err != nil {
		t.Fatal(err)
	}
	create := func(id string) ManualSessionCreationView {
		v, err := a.BeginManualSessionCreation(ManualSessionCreationRequest{OperationID: id, Scope: "project", WorkspaceRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		a.manualCreationTasks.Wait()
		v, err = a.GetManualSessionCreation(v.OperationID)
		if err != nil || v.Phase != "ready" {
			t.Fatalf("creation: %+v %v", v, err)
		}
		if _, err := a.OpenSession(v.Ref); err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := create("boot-multi-first")
	firstCtrl := a.activeCtrl().(*control.Controller)
	t.Cleanup(func() { firstCtrl.Close(); <-firstCtrl.Closed() })
	a.SetToolApprovalModeForTab(a.activeTab().ID, control.ToolApprovalDangerFullAccess)
	if _, err := a.StartTurnForTab(a.activeTab().ID, "hold-first-session", "boot-first-send"); err != nil {
		t.Fatal(err)
	}
	if longTool {
		deadline := time.Now().Add(15 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			if time.Now().After(deadline) {
				for _, message := range firstCtrl.History() {
					if message.Role == "tool" {
						t.Logf("%s: %s", message.Name, message.Content)
					}
				}
				t.Fatal("shell tool did not start")
			}
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("provider did not start")
		}
	}
	// The renderer refreshes tab metadata and runtime projections while it
	// navigates; exercise those reads concurrently with terminal publication.
	stopReads, readsDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readsDone)
		for {
			select {
			case <-stopReads:
				return
			default:
				a.ListTabs()
				a.GetRuntimeStateSnapshot()
			}
		}
	}()
	defer func() { close(stopReads); <-readsDone }()
	second := create("boot-multi-second")
	secondCtrl := a.activeCtrl().(*control.Controller)
	t.Cleanup(func() { secondCtrl.Close(); <-secondCtrl.Closed() })
	tabID := a.activeTab().ID
	input, err := a.GetSessionComposerState(second.Ref)
	if err != nil {
		t.Fatal(err)
	}
	input, err = a.SaveSessionComposerState(SessionComposerSaveRequest{Ref: second.Ref, ExpectedRevision: input.Revision, ContentJSON: `{"text":"你是谁"}`, ContentVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.BeginSessionComposerSubmission(second.Ref, input.Revision, "boot-second-send", `{"kind":"turn"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartTurnForTab(tabID, "你是谁", "boot-second-send"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CompleteSessionComposerSubmission(second.Ref, "boot-second-send", "accepted"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.OpenSession(first.Ref); err != nil {
		t.Fatal(err)
	}
	if done, busy := secondCtrl.TurnIdleDone(); busy {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("second session stuck finishing")
		}
	}
	if !firstCtrl.Running() {
		t.Fatal("sibling was stopped")
	}
	if _, err := a.OpenSession(second.Ref); err != nil {
		t.Fatal(err)
	}
	if input, err := a.GetSessionComposerState(second.Ref); err != nil || input.SubmissionID != "" {
		t.Fatalf("input blocked: %+v %v", input, err)
	}
	releaseProvider()
	if done, busy := firstCtrl.TurnIdleDone(); busy {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("first session did not finish after release")
		}
	}
}
