package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"deepzora/internal/config"
	"deepzora/internal/provider"
)

// Discover before taking config/credential edit locks. A failed or empty
// gateway catalog leaves the connection and credential store untouched.
func (a *App) discoverMagpieConnection(presetID, sourceName, key, baseURL, kind string) ([]provider.ModelInfo, error) {
	if presetID != "magpie" || sourceName != "" {
		return nil, nil
	}
	if kind != "" && kind != "openai" && kind != "responses" && kind != "anthropic" {
		return nil, fmt.Errorf("invalid provider protocol")
	}
	preset, _ := config.CuratedProviderPreset("magpie")
	entry := preset.Entries[0]
	applyConnectionOverrides(&entry, kind, strings.TrimSpace(baseURL), config.CatalogForProviderPreset(preset).Protocols)
	// A new connection never borrows another connection's saved credential.
	entry = entry.WithAPIKeyForProbe(strings.TrimSpace(key))
	ctx, cancel := context.WithTimeout(a.reqCtx(), 15*time.Second)
	defer cancel()
	catalog, err := entry.FetchModelCatalogWithProxy(ctx, withProbeDirectHost(a.networkProxySpecForRoot(a.activeWorkspaceRoot()), entry.BaseURL, entry.NoProxy))
	if err != nil {
		return nil, fmt.Errorf("Magpie model discovery failed: %w", err)
	}
	chat := make([]provider.ModelInfo, 0, len(catalog))
	for _, model := range catalog {
		if config.IsLikelyChatModel(model.ID) {
			chat = append(chat, model)
		}
	}
	if len(chat) == 0 {
		return nil, fmt.Errorf("Magpie returned no chat models; configure its providers and try again")
	}
	return chat, nil
}

// Cache labels only after the new credential/config transaction has committed,
// so the cache fingerprint belongs to the installed connection and credential.
func cacheConnectionCatalog(entries []config.ProviderEntry, catalog []provider.ModelInfo) {
	if len(catalog) == 0 {
		return
	}
	resolver := config.NewModelCapabilityResolver()
	for _, entry := range entries {
		resolver.PutCatalog(entry, catalog)
	}
}
