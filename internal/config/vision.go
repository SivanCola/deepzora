package config

import (
	"strings"

	"deepzora/internal/provider/openai"
)

// VisionCapability is the model-level image-input fact used by settings and
// turn preparation. Unknown is deliberately distinct from unsupported: an
// unknown model is kept safe by the text-only path without claiming that the
// provider can never accept images.
type VisionCapability string

const (
	VisionCapabilityUnknown     VisionCapability = "unknown"
	VisionCapabilitySupported   VisionCapability = "supported"
	VisionCapabilityUnsupported VisionCapability = "unsupported"
)

// CanConfigureVision is retained for the desktop payload contract. Capability
// choices are now derived from model metadata; the wire layer still refuses
// unsupported official DeepSeek Flash/Pro image payloads.
func CanConfigureVision(e *ProviderEntry) bool {
	return e != nil
}

// AllowsImages reports whether image input may be sent to the model itself:
// declared supported, or undeclared with no fallback model to route through.
// Only a declared text-only model is blocked.
func (v VisionCapability) AllowsImages(fallbackConfigured bool) bool {
	return v == VisionCapabilitySupported || (v == VisionCapabilityUnknown && !fallbackConfigured)
}

// EffectiveVision resolves whether the selected model accepts image input.
// Official DeepSeek Settings can mark any enabled model for image input, but
// only the pinned vision SKU actually receives image parts. An unconfigured
// catalog (VisionModels == nil) still enables that SKU so CLI/model-switch
// users keep image input without a Settings round-trip. Explicit provider
// vision still wins for custom gateways; the MiMo endpoint heuristic is
// deliberately limited to known MiMo endpoints so arbitrary OpenAI-compatible
// proxies do not get image payloads unexpectedly.
func EffectiveVision(e *ProviderEntry) bool {
	return VisionCapabilityForModel(e) == VisionCapabilitySupported
}

// VisionCapabilityForModel uses the current model resolver and explicit overrides.
func VisionCapabilityForModel(e *ProviderEntry) VisionCapability {
	if e == nil {
		return VisionCapabilityUnknown
	}
	resolved := NewModelCapabilityResolver().Resolve(e)
	switch resolved.State {
	case CapabilitySupported:
		return VisionCapabilitySupported
	case CapabilityUnsupported:
		return VisionCapabilityUnsupported
	}
	return VisionCapabilityUnknown
}

// ExplicitModelVision reports whether the selected model has an explicit,
// positive image capability declaration that the endpoint is allowed to use.
// Keep this query separate from EffectiveVision so callers can distinguish a
// model-scoped capability from provider-wide or endpoint-inferred support.
func ExplicitModelVision(e *ProviderEntry) bool {
	if e == nil {
		return false
	}
	if openai.IsDeepSeek(e.BaseURL) {
		return officialDeepSeekEffectiveVision(e)
	}
	enabled, explicit := explicitModelVision(e)
	return explicit && enabled
}

func officialDeepSeekEffectiveVision(e *ProviderEntry) bool {
	if e == nil || openai.IsOfficialDeepSeekTextModel(e.Model) {
		return false
	}
	if enabled, explicit := explicitModelVision(e); explicit {
		return enabled
	}
	if !openai.IsOfficialDeepSeekImageModel(e.Model) {
		return false
	}
	return true
}

func explicitModelVision(e *ProviderEntry) (enabled, explicit bool) {
	if e == nil {
		return false, false
	}
	if ov, ok := e.modelOverrideForModel(e.Model); ok && ov.Vision != nil {
		return *ov.Vision, true
	}
	if e.visionOverride != nil {
		return *e.visionOverride, true
	}
	return false, false
}

func (e *ProviderEntry) HasVisionModel(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, candidate := range e.VisionModels {
		if strings.TrimSpace(candidate) == model {
			return true
		}
	}
	return false
}
