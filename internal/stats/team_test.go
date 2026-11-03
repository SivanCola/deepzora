package stats

import (
	"deepzora/internal/billing"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"testing"
	"time"
)

func TestTeamUsageAttributionCountsEachCallOnceAndRetainsUnpricedAndMixedCurrencies(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(event.Discard, dir, "desktop")
	owned := event.NewTeamUsageSink(r, event.TeamUsageAttribution{AgentID: "agent-a", GroupID: "group", SeatID: "seat"})
	for _, currency := range []string{"USD", "CNY"} {
		e := event.Event{Kind: event.Usage, ModelRef: "test/model", Usage: &provider.Usage{TotalTokens: 10, PromptTokens: 10, RequestCount: 1}, CostQuote: &billing.CostQuote{Original: billing.Money{Amount: "1", Currency: currency}, CostComplete: true}}
		owned.Emit(e)
	}
	owned.Emit(event.Event{Kind: event.Usage, UsageSource: event.UsageSourceMemoryReview, ModelRef: "test/unpriced", Usage: &provider.Usage{RequestCount: 1}})
	r.Emit(event.Event{Kind: event.Usage, ModelRef: "legacy/model", Usage: &provider.Usage{TotalTokens: 3, RequestCount: 1}})
	flushRecorder(t, r)
	filter := SourceFilter{From: time.Now(), To: time.Now()}
	rows, err := NewWriter(dir).QueryTeam(filter)
	if err != nil {
		t.Fatal(err)
	}
	total := int64(0)
	requests := 0
	unattributed := false
	mixed := false
	unpriced := false
	for _, row := range rows {
		total += row.Tokens
		requests += row.Requests
		if row.AgentID == "" {
			unattributed = true
		}
		if row.Model == "test/model" {
			mixed = len(row.Cost.OriginalTotals) == 2
		}
		if row.UsageSource == event.UsageSourceMemoryReview {
			unpriced = !row.Cost.CostComplete && row.Cost.IncompleteReason != ""
		}
	}
	if total != 23 || requests != 4 || !unattributed || !mixed || !unpriced {
		t.Fatalf("Team totals lost accounting facts: %+v", rows)
	}
	ordinary, err := NewWriter(dir).Query(filter)
	if err != nil || ordinary.Tokens != total || ordinary.Requests != requests {
		t.Fatalf("Team totals disagree with global totals: %+v %v", ordinary, err)
	}
}
