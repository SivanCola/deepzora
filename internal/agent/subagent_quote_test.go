package agent

import (
	"context"
	"deepzora/internal/billing"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"testing"
	"time"
)

func TestSubagentSkillOptionsPreserveScheduledQuoteContext(t *testing.T) {
	ratedAt := time.Date(2026, time.August, 17, 4, 0, 0, 0, time.UTC)
	quoteCtx := &event.QuoteContext{
		Now: func() time.Time { return ratedAt },
		PricingContextForModel: func(modelRef string) billing.PricingContext {
			if modelRef != "deepseek-pro/deepseek-v4-pro" {
				t.Fatalf("pricing context model = %q", modelRef)
			}
			return billing.PricingContext{
				ProviderKind: "deepseek",
				ModelID:      "deepseek-v4-pro",
				BillingMode:  billing.BillingModePAYG,
				ScheduleID:   billing.ScheduleDeepSeekV4August2026,
			}
		},
	}
	runner := NewSubagentRunner(SubagentRunnerOptions{QuoteContext: quoteCtx})
	opts := runner.subagentOptions(context.Background(), 5, &provider.Pricing{CacheHit: 0.30, Input: 9, Output: 27, Currency: "CNY"}, 1_000_000, 1, "", nil)
	if opts.QuoteContext != quoteCtx {
		t.Fatal("skill subagent did not retain the host QuoteContext")
	}

	quote := event.EnsureCostQuote(event.Event{
		Kind:        event.Usage,
		ModelRef:    "deepseek-pro/deepseek-v4-pro",
		Usage:       &provider.Usage{PromptTokens: 1_000_000, TotalTokens: 1_000_000},
		Pricing:     opts.Pricing,
		UsageSource: event.UsageSourceSubagent,
	}, opts.QuoteContext)
	if quote == nil || quote.RateBand != billing.RateBandOffPeak || quote.Original.Amount != "4.5" {
		t.Fatalf("skill subagent quote = %+v, want CNY 4.5 off-peak", quote)
	}
}
