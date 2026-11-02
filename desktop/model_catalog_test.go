package main

import (
	"testing"

	"deepzora/internal/config"
)

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
