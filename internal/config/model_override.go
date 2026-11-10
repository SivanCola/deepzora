package config

type ProviderModelOverride struct {
	// DisplayName is a user-chosen label shown in place of the model ID. The ID
	// stays the routing identity: the label never reaches a provider request,
	// the prompt, or a session's stored model reference.
	DisplayName       string   `toml:"display_name"`
	ReasoningProtocol string   `toml:"reasoning_protocol"`
	SupportedEfforts  []string `toml:"supported_efforts"`
	DefaultEffort     string   `toml:"default_effort"`
	Vision            *bool    `toml:"vision"`
	// WebSearch overrides ProviderEntry.WebSearch for this model. Nil inherits
	// the connection. It cannot add a capability the wire protocol lacks:
	// SupportsServerWebSearch still gates the provider request.
	WebSearch *bool `toml:"web_search"`
	// ContextWindow overrides the provider-wide context budget for this model.
	// Zero inherits ProviderEntry.ContextWindow so existing configurations keep
	// their current compaction behavior.
	ContextWindow int `toml:"context_window"`
	// MaxOutputTokens overrides the provider-wide output budget. Zero inherits;
	// positive values set a cap and negative values omit optional wire limits.
	MaxOutputTokens int `toml:"max_output_tokens"`
}

// modelOverridesWithoutLabels drops the UI-only label from every override, so a
// renamed model is not mistaken for a changed runtime. Overrides left with
// nothing else are removed, as the loader would remove them.
func modelOverridesWithoutLabels(in map[string]ProviderModelOverride) map[string]ProviderModelOverride {
	var out map[string]ProviderModelOverride
	for model, ov := range in {
		ov.DisplayName = ""
		if modelOverrideEmpty(ov) {
			continue
		}
		if out == nil {
			out = make(map[string]ProviderModelOverride, len(in))
		}
		out[model] = ov
	}
	return out
}
