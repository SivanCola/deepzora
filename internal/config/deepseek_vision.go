package config

import (
	"deepzora/internal/provider"
	"deepzora/internal/provider/openai"
)

func backfillOfficialDeepSeekVisionPrice(p *ProviderEntry) {
	sku := openai.OfficialDeepSeekVisionModel
	if p == nil || !p.HasModel(sku) {
		return
	}
	if price, ok := pricingForModelKey(p.Prices, sku); ok && price != nil {
		return
	}
	currency := p.ProviderBillingCurrency()
	if currency == "" {
		currency = "USD"
	}
	price := deepSeekV4PriceForModel(currency, sku)
	if price == nil {
		return
	}
	if p.Prices == nil {
		p.Prices = map[string]*provider.Pricing{}
	}
	p.Prices[sku] = price
}
