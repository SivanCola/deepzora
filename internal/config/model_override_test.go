package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// The label is the only override some models carry, so every "is this override
// empty" decision must count it or the loader and renderer drop it (#137).
func TestModelDisplayNameSurvivesLoadAndRender(t *testing.T) {
	entry := ProviderEntry{Name: "p", Kind: "openai", Models: []string{"deepseek-flash-1017", "other"},
		ModelOverrides: map[string]ProviderModelOverride{
			"deepseek-flash-1017": {DisplayName: "  deepseek-flash  "},
			"other":               {DisplayName: "   "},
		}}
	normalized := normalizedModelOverrides(entry.ModelOverrides)
	if len(normalized) != 1 || normalized["deepseek-flash-1017"].DisplayName != "deepseek-flash" {
		t.Fatalf("normalized overrides = %+v", normalized)
	}
	rendered := renderModelOverrides(normalized)
	if !strings.Contains(rendered, `"deepseek-flash-1017" = { display_name = "deepseek-flash" }`) {
		t.Fatalf("rendered overrides = %s", rendered)
	}
	var decoded struct {
		ModelOverrides map[string]ProviderModelOverride `toml:"model_overrides"`
	}
	if err := toml.Unmarshal([]byte("model_overrides = "+rendered+"\n"), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ModelOverrides["deepseek-flash-1017"].DisplayName != "deepseek-flash" {
		t.Fatalf("decoded overrides = %+v", decoded.ModelOverrides)
	}
}

func TestModelDisplayNameIsTheResolvedModelName(t *testing.T) {
	c := &Config{Providers: []ProviderEntry{{Name: "p", Kind: "openai", Models: []string{"deepseek-flash-1017", "other"},
		ModelOverrides: map[string]ProviderModelOverride{"deepseek-flash-1017": {DisplayName: "deepseek-flash"}}}}}
	for model, want := range map[string]string{"deepseek-flash-1017": "deepseek-flash", "other": ""} {
		entry, ok := c.ResolveModel("p/" + model)
		if !ok {
			t.Fatalf("%s did not resolve", model)
		}
		if entry.Model != model {
			t.Fatalf("label changed the routing model: %q", entry.Model)
		}
		if got := NewModelCapabilityResolver().Resolve(entry).ModelInfo.Name; got != want {
			t.Fatalf("%s name = %q, want %q", model, got, want)
		}
	}
}

// A label is display metadata: renaming a model must not look like a changed
// runtime, or every open session would be rebuilt.
func TestModelDisplayNameDoesNotChangeTheRuntimeFingerprint(t *testing.T) {
	build := func(overrides map[string]ProviderModelOverride) *Config {
		c := Default()
		c.Providers = []ProviderEntry{{Name: "p", Kind: "openai", Models: []string{"m"}, ModelOverrides: overrides}}
		c.FreezeProviderCredentials()
		return c
	}
	plain := build(nil).ModelRuntimeFingerprint("p/m")
	if build(map[string]ProviderModelOverride{"m": {DisplayName: "Mine"}}).ModelRuntimeFingerprint("p/m") != plain {
		t.Fatal("a label changed the runtime fingerprint")
	}
	capped := build(map[string]ProviderModelOverride{"m": {MaxOutputTokens: 100}}).ModelRuntimeFingerprint("p/m")
	if capped == plain {
		t.Fatal("an output cap was not detected")
	}
	if build(map[string]ProviderModelOverride{"m": {DisplayName: "Mine", MaxOutputTokens: 100}}).ModelRuntimeFingerprint("p/m") != capped {
		t.Fatal("a label changed the fingerprint of a capped model")
	}
}
