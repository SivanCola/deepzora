package stats

import (
	"deepzora/internal/billing"
	"slices"
	"strings"
)

type TeamUsage struct {
	AgentID     string            `json:"agentId"`
	GroupID     string            `json:"groupId,omitempty"`
	SeatID      string            `json:"seatId,omitempty"`
	Model       string            `json:"model"`
	UsageSource string            `json:"usageSource,omitempty"`
	Tokens      int64             `json:"tokens"`
	Requests    int               `json:"requests"`
	CacheHit    int64             `json:"cacheHit"`
	CacheMiss   int64             `json:"cacheMiss"`
	Cost        billing.CostQuote `json:"cost"`
}

// QueryTeam reads authoritative rows so it works with old/rebuildable indexes.
// Empty AgentID is retained as unattributed; it never guesses old ownership.
func (w *Writer) QueryTeam(filter SourceFilter) ([]TeamUsage, error) {
	result := []TeamUsage{}
	if w == nil || w.dir == "" {
		return result, nil
	}
	rows, err := readDailyRange(w.dir, daysInRange(filter.From, filter.To))
	if err != nil {
		return nil, err
	}
	type key struct{ agent, group, seat, model, source string }
	groups := map[key]*TeamUsage{}
	quotes := map[key][]billing.CostQuote{}
	for _, records := range rows {
		for _, record := range records {
			if record.Turn || !matchesSource(record.Source, filter.Source) {
				continue
			}
			k := key{record.AgentID, record.GroupID, record.SeatID, record.ModelRef, record.UsageSource}
			total := groups[k]
			if total == nil {
				total = &TeamUsage{AgentID: k.agent, GroupID: k.group, SeatID: k.seat, Model: k.model, UsageSource: k.source}
				groups[k] = total
			}
			total.Tokens += int64(record.Total)
			total.Requests += record.Requests
			total.CacheHit += int64(record.CacheHit)
			total.CacheMiss += int64(record.CacheMiss)
			quotes[k] = append(quotes[k], record.quote())
		}
	}
	for k, total := range groups {
		total.Cost = billing.AggregateQuotes(quotes[k], "")
		result = append(result, *total)
	}
	slices.SortFunc(result, func(a, b TeamUsage) int {
		return strings.Compare(a.AgentID+"\x00"+a.GroupID+"\x00"+a.SeatID+"\x00"+a.Model+"\x00"+a.UsageSource, b.AgentID+"\x00"+b.GroupID+"\x00"+b.SeatID+"\x00"+b.Model+"\x00"+b.UsageSource)
	})
	return result, nil
}
func (r record) quote() billing.CostQuote {
	q := billing.CostQuote{Original: billing.Money{Amount: r.CostAmount, Currency: r.CostCurrency}, Estimated: r.CostEstimated, IncompleteReason: r.IncompleteReason, BillingMode: r.BillingMode, ModelRef: r.ModelRef, UsageSource: r.UsageSource, PricingFingerprint: r.PricingFingerprint, RateDate: r.RateDate, RateBand: r.RateBand, RatedAt: r.RatedAt, DisplayStatus: r.DisplayStatus, AggregateMode: r.AggregateMode}
	if r.CostComplete != nil {
		q.CostComplete = *r.CostComplete
	}
	if r.DisplayComplete != nil {
		q.DisplayComplete = *r.DisplayComplete
		q.Complete = *r.DisplayComplete
	}
	if r.CostComplete == nil {
		q.IncompleteReason = "pricing_missing"
		q.Estimated = true
	}
	if r.SelectedCurrency != "" {
		q.Selected = &billing.Money{Amount: r.SelectedAmount, Currency: r.SelectedCurrency}
	}
	for _, value := range r.OriginalTotals {
		currency, amount, ok := strings.Cut(value, ":")
		if ok {
			q.OriginalTotals = append(q.OriginalTotals, billing.Money{Currency: currency, Amount: amount})
		}
	}
	q.Valuations = map[string]billing.Valuation{}
	for currency, amount := range map[string]string{"CNY": r.ValuationCNY, "USD": r.ValuationUSD} {
		if amount != "" {
			q.Valuations[currency] = billing.Valuation{Money: billing.Money{Amount: amount, Currency: currency}, AsOf: r.RateDate}
		}
	}
	return q
}
