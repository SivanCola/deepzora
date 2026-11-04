package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"deepzora/internal/config"
	"deepzora/internal/event"
	"deepzora/internal/stats"
	"deepzora/internal/team"
)

func TestTeamReviewModelCleanupFlushesOwnedRealHTTPUsage(t *testing.T) {
	a, id := newMemoryTeamApp(t)
	t.Cleanup(func() { _ = stats.CloseRecordDispatchers(context.Background()) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"changes\\\":[]}\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":2,\"total_tokens\":13}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "review-usage", Kind: "openai", BaseURL: server.URL, Models: []string{"test"}, NoProxy: true}, "isolated-review-key"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("review-usage/test"); err != nil {
		t.Fatal(err)
	}
	ask, closeProvider, err := a.teamMemoryReviewer(t.Context(), team.SeatRef{Agent: id, Group: "group"}, "review-seat", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ask(t.Context(), "review", "human evidence"); err != nil {
		closeProvider()
		t.Fatal(err)
	}
	closeProvider()
	rows, err := stats.NewWriter(config.StatsDir()).QueryTeam(stats.SourceFilter{From: time.Now(), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AgentID != id || rows[0].GroupID != "group" || rows[0].SeatID != "review-seat" || rows[0].UsageSource != event.UsageSourceMemoryReview || rows[0].Requests != 1 || rows[0].Tokens != 13 {
		t.Fatalf("review usage missing, duplicated or unowned after cleanup: %+v", rows)
	}
}
