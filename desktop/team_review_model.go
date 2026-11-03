package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"deepzora/internal/boot"
	"deepzora/internal/boundedllm"
	"deepzora/internal/config"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/stats"
	"deepzora/internal/team"
)

func (a *App) teamMemoryReviewer(ctx context.Context, ref team.SeatRef, sessionID, root, model string) (team.Reviewer, func(), error) {
	cfg, err := config.LoadForRoot(root)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(model) == "" {
		model = strings.TrimSpace(cfg.DefaultModel)
	}
	if model == "" {
		return nil, nil, fmt.Errorf("no default chat or memory review model is configured")
	}
	handle, err := boot.AcquireAuxiliaryProvider(ctx, boot.AuxiliaryProviderRequest{Config: cfg, SessionID: sessionID, WorkspaceRoot: root, ModelRef: model})
	if err != nil {
		return nil, nil, err
	}
	prov, err := handle.Resolver.Resolve(provider.Selection{Ref: model})
	if err != nil {
		_ = handle.Close()
		return nil, nil, err
	}
	var price *provider.Pricing
	if entry, ok := cfg.ResolveModel(model); ok {
		price = entry.Price
	}
	recorder := stats.NewRecorder(event.Discard, config.StatsDir(), "desktop")
	owned := event.FuncSink(func(e event.Event) {
		e.Team = &event.TeamUsageAttribution{AgentID: ref.Agent, GroupID: ref.Group, SeatID: sessionID}
		recorder.Emit(e)
	})
	sink := event.NewCostQuoteSink(owned, boot.QuoteContextFor(cfg))
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		return boundedllm.Call(ctx, boundedllm.Config{Provider: prov, ModelRef: model, Pricing: price, Sink: sink, UsageSource: event.UsageSourceMemoryReview,
			Timeout: 2 * time.Minute, MaxTokens: 2048, MaxOutputBytes: 32 * 1024, MaxSystemBytes: 4096, MaxTotalBytes: 24 * 1024, EffortOverride: provider.PreferredReasoning(prov, "low")}, system, prompt)
	}
	return ask, func() { _ = handle.Close() }, nil
}
