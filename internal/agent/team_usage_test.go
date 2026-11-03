package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"deepzora/internal/event"
	"deepzora/internal/jobs"
	"deepzora/internal/provider"
	"deepzora/internal/stats"
)

func TestTaskSubagentUsageRetainsTeamOwnerWithoutDuplicateRecords(t *testing.T) {
	t.Cleanup(func() { _ = stats.CloseRecordDispatchers(context.Background()) })
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprint(background), func(t *testing.T) {
			dir := t.TempDir()
			recorder := stats.NewRecorder(event.Discard, dir, "desktop")
			sink := event.NewTeamUsageSink(recorder, event.TeamUsageAttribution{AgentID: "member", GroupID: "group", SeatID: "seat"})
			prov := &scriptedProvider{name: "child", turns: [][]provider.Chunk{{
				{Type: provider.ChunkText, Text: "done"},
				{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, RequestCount: 1}},
				{Type: provider.ChunkDone},
			}}}
			task := NewTaskToolWithOptions(TaskToolOptions{Provider: prov, ParentRegistry: evidenceRegistry(), MaxSteps: 4, SysPrompt: "sys"}).WithTranscripts(newTestSubagentStore(t, t.TempDir()), t.TempDir(), "child/model", "")
			manager := jobs.NewManager(event.Discard)
			defer manager.Close()
			ctx := withCallContext(testTaskContext(), "parent-call", sink, nil, false)
			ctx = jobs.WithManager(jobs.WithSession(ctx, "parent-session"), manager)
			out, err := task.Execute(ctx, fmt.Appendf(nil, `{"prompt":"answer","run_in_background":%t}`, background))
			if err != nil {
				t.Fatal(err)
			}
			if background {
				results := manager.WaitForSession(context.Background(), "parent-session", []string{extractJobID(out)}, 5)
				if len(results) != 1 || results[0].Status != jobs.Done {
					t.Fatalf("background task: %+v", results)
				}
			}
			if err := recorder.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			rows, err := stats.NewWriter(dir).QueryTeam(stats.SourceFilter{From: time.Now(), To: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].AgentID != "member" || rows[0].GroupID != "group" || rows[0].SeatID != "seat" || rows[0].UsageSource != event.UsageSourceSubagent || rows[0].Tokens != 13 || rows[0].Requests != 1 {
				t.Fatalf("child usage lost owner or double counted: %+v", rows)
			}
		})
	}
}
