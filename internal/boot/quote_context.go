package boot

import (
	"deepzora/internal/billing"
	"deepzora/internal/config"
	"deepzora/internal/event"
)

// QuoteContextFor keeps auxiliary calls on the same occurrence-time currency,
// billing mode and pricing identity contract as normal session calls.
func QuoteContextFor(cfg *config.Config) *event.QuoteContext {
	return &event.QuoteContext{DisplayRequest: billing.DisplayRequest{Currency: cfg.ExplicitDisplayCurrency(), Source: billing.DisplaySourceExplicit},
		BillingModeForModel: func(ref string) string {
			entry, ok := cfg.ResolveModel(ref)
			if !ok {
				return ""
			}
			return entry.ProviderBillingMode()
		},
		PricingContextForModel: func(ref string) billing.PricingContext {
			entry, ok := cfg.ResolveModel(ref)
			if !ok {
				return billing.PricingContext{}
			}
			return entry.PricingContextForModel(entry.Model)
		},
	}
}
