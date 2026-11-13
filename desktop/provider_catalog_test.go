package service

import (
	"deepzora/internal/config"
	"strings"
	"testing"
)

func TestProviderCatalogViewsAndInstallPreserveExistingConnections(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	if _, err := a.AddProviderPresetAccess("glm-coding-plan-cn", ""); err != nil {
		t.Fatal(err)
	}
	before := config.LoadForEdit(config.UserConfigPath())
	original, ok := before.Provider("glm-coding-plan-cn")
	if !ok {
		t.Fatal("missing original")
	}
	if _, err := a.AddProviderPresetAccess("openrouter", ""); err != nil {
		t.Fatal(err)
	}
	after := config.LoadForEdit(config.UserConfigPath())
	current, ok := after.Provider(original.Name)
	if !ok || !config.ProviderEntriesConfigEqual(*original, *current) {
		t.Fatal("adding another brand changed an existing connection")
	}
	views := a.Settings().ProviderPresets
	found := false
	for _, view := range views {
		if view.ID == "glm-coding-plan-cn" {
			found = true
			if view.Catalog.BrandID != "zai" || view.Catalog.Product != "coding" || view.Catalog.Format != "openai" || view.Status != "installed" {
				t.Fatalf("incorrect catalog view: %+v", view.Catalog)
			}
		}
	}
	if !found {
		t.Fatal("missing installed route")
	}
}

func TestMagpieConnectionsUseCatalogProtocolsAndIndependentKeys(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	for _, tc := range []struct{ kind, baseURL, requestURL string }{
		{"openai", "http://127.0.0.1:3425/v1", "http://127.0.0.1:3425/v1/chat/completions"},
		{"responses", "http://127.0.0.1:3425/v1", "http://127.0.0.1:3425/v1/responses"},
		{"anthropic", "http://127.0.0.1:3425", "http://127.0.0.1:3425/v1/messages"},
	} {
		if _, err := a.applyModelConfigChangeWithWarning("provider access", func(c *config.Config) error {
			return addProviderConnectionConfig(c, "magpie", "", "magpie-deepzora", tc.baseURL, tc.kind)
		}); err != nil {
			t.Fatal(err)
		}
		cfg, _, err := a.loadDesktopUserConfigForView()
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, p := range cfg.Providers {
			if p.PresetID != "magpie" || p.Kind != tc.kind {
				continue
			}
			found = true
			if config.ProviderEffectiveRequestURL(&p) != tc.requestURL || !p.NoProxy || p.AuthHeader != (tc.kind == "anthropic") {
				t.Fatalf("incorrect Magpie protocol/network settings: %+v", p)
			}
			if p.APIKeyEnv == "MAGPIE_API_KEY" || p.APIKey() != "magpie-deepzora" {
				t.Fatal("Magpie connection did not stage its own credential")
			}
			if p.DefaultModel() != "deepseek/deepseek-v4-pro" {
				t.Fatalf("upstream example model was not preserved: %s", p.DefaultModel())
			}
			if _, ok := cfg.ResolveModel(p.Name + "/" + p.DefaultModel()); !ok {
				t.Fatal("namespaced gateway model did not resolve")
			}
			candidates, err := config.BuildModelFetchURLs(p.BaseURL, p.ModelsURL)
			if err != nil || len(candidates) == 0 || !strings.Contains(strings.Join(candidates, " "), "http://127.0.0.1:3425/v1/models") {
				t.Fatalf("Magpie catalog unreachable: %v, %v", candidates, err)
			}
		}
		if !found {
			t.Fatalf("missing Magpie %s connection", tc.kind)
		}
	}
	preset, _ := config.CuratedProviderPreset("magpie")
	if preset.Entries[0].Kind != "openai" || preset.Entries[0].APIKeyEnv != "MAGPIE_API_KEY" {
		t.Fatal("installing connections mutated the preset")
	}
}
