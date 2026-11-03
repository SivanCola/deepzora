package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/stats"
)

func TestSpawnSubagentUsageRetainsTeamOwnerWithoutDuplicateRecords(t *testing.T) {
	t.Cleanup(func() { _ = stats.CloseRecordDispatchers(context.Background()) })
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(readOnly), func(t *testing.T) {
			dir := t.TempDir()
			recorder := stats.NewRecorder(event.Discard, dir, "desktop")
			sink := event.NewTeamUsageSink(recorder, event.TeamUsageAttribution{AgentID: "member", GroupID: "group", SeatID: "seat"})
			prov := &scriptedProvider{name: "child", turns: [][]provider.Chunk{{
				{Type: provider.ChunkText, Text: "done"},
				{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, RequestCount: 1}},
				{Type: provider.ChunkDone},
			}}}
			runner := NewSubagentRunner(SubagentRunnerOptions{Provider: prov, ParentRegistry: evidenceRegistry(), MaxSteps: 4, SysPrompt: "sys"}).WithTranscripts(newTestSubagentStore(t, t.TempDir()), t.TempDir(), "child/model", "")
			task := NewSpawner(runner)
			ctx := withCallContext(testTaskContext(), "parent-call", sink, nil, false)
			_, err := task.Execute(ctx, fmt.Appendf(nil, `{"tasks":[{"title":"answer","task":"answer","read_only":%t}]}`, readOnly))
			if err != nil {
				t.Fatal(err)
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
