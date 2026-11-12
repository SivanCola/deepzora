package service

import (
	"testing"

	"deepzora/internal/config"
)

func TestDesktopModelLabelStableAcrossBoot(t *testing.T) {
	cfg := &config.Config{Providers: []config.ProviderEntry{{Name: "long-connection-id", Kind: "openai", Models: []string{"org/custom-model", "deepseek-v4-flash"}}}}
	for _, model := range cfg.Providers[0].Models {
		ref := "long-connection-id/" + model
		cold := desktopModelLabel(cfg, ref, "")
		ready := desktopModelLabel(cfg, ref, model)
		info := configuredModelInfo(cfg, "long-connection-id", model, true)
		want := info.ModelDisplayName
		if want == "" {
			want = model
		}
		if cold != want || ready != want {
			t.Fatalf("%s: cold=%q ready=%q want=%q", ref, cold, ready, want)
		}
	}
	if got := desktopModelLabel(cfg, "plugin/example/org/custom-model", "extension-owned"); got != "extension-owned" {
		t.Fatalf("extension label changed: %q", got)
	}
	if got := desktopModelLabel(cfg, "long-connection-id/org/custom-model", "org/custom-model + planner custom"); got != "org/custom-model + planner custom" {
		t.Fatalf("composite label changed: %q", got)
	}
}

func TestConfiguredModelInfoPreservesPresetBrand(t *testing.T) {
	for _, tc := range []struct {
		name, preset, brand string
	}{
		{"renamed-gateway", "magpie", "magpie"},
		{"openai-work", "magpie", "magpie"},
		{"renamed-vendor", "deepseek-chat", "deepseek"},
		{"custom", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Providers: []config.ProviderEntry{{
				Name: tc.name, DisplayName: "My connection", PresetID: tc.preset,
				Kind: "openai", Models: []string{"upstream/model"},
			}}}
			info := configuredModelInfo(cfg, tc.name, "upstream/model", true)
			if info.BrandID != tc.brand {
				t.Fatalf("brand = %q, want %q", info.BrandID, tc.brand)
			}
			if info.Provider != tc.name || info.Ref != tc.name+"/upstream/model" || info.DisplayName != "My connection" || !info.Current {
				t.Fatalf("brand projection changed connection identity: %+v", info)
			}
		})
	}
}

func TestConfiguredModelInfoBrandsAllPresets(t *testing.T) {
	for _, preset := range config.CuratedProviderPresets() {
		t.Run(preset.ID, func(t *testing.T) {
			catalog := config.CatalogForProviderPreset(preset)
			for _, entry := range preset.Entries {
				entry.Name = "renamed-connection"
				entry.DisplayName = "My connection"
				entry.Models = []string{"upstream/model"}
				cfg := &config.Config{Providers: []config.ProviderEntry{entry}}
				info := configuredModelInfo(cfg, entry.Name, "upstream/model", false)
				if info.BrandID != catalog.BrandID {
					t.Fatalf("preset %s (%s): brand = %q, want %q", preset.ID, entry.Kind, info.BrandID, catalog.BrandID)
				}
			}
		})
	}
}

func TestConfiguredModelInfoUsesTheModelDisplayName(t *testing.T) {
	cfg := &config.Config{Providers: []config.ProviderEntry{{Name: "p", Kind: "openai", Models: []string{"deepseek-flash-1017", "other"},
		ModelOverrides: map[string]config.ProviderModelOverride{"deepseek-flash-1017": {DisplayName: "deepseek-flash"}}}}}
	info := configuredModelInfo(cfg, "p", "deepseek-flash-1017", true)
	if info.ModelDisplayName != "deepseek-flash" || info.Model != "deepseek-flash-1017" || info.Ref != "p/deepseek-flash-1017" {
		t.Fatalf("labelled model = %+v", info)
	}
	if got := desktopModelLabel(cfg, "p/deepseek-flash-1017", "deepseek-flash-1017"); got != "deepseek-flash" {
		t.Fatalf("session label = %q", got)
	}
	if other := configuredModelInfo(cfg, "p", "other", false); other.ModelDisplayName != "" {
		t.Fatalf("label leaked to another model: %+v", other)
	}
	view := providerModelOverridesForView(cfg.Providers[0].ModelOverrides, cfg.Providers[0].Models)
	if len(view) != 1 || view[0].DisplayName != "deepseek-flash" {
		t.Fatalf("override view = %+v", view)
	}
	saved := providerModelOverridesForSave([]ProviderModelOverrideView{{Model: "deepseek-flash-1017", DisplayName: " deepseek-flash "}, {Model: "other", DisplayName: "  "}}, cfg.Providers[0].Models)
	if len(saved) != 1 || saved["deepseek-flash-1017"].DisplayName != "deepseek-flash" {
		t.Fatalf("saved overrides = %+v", saved)
	}
}
