package service

import (
	"log/slog"
	"strings"
	"time"

	"deepzora/internal/config"
	"deepzora/internal/control"
	"deepzora/internal/provider"
)

// Use the same configured name before and after boot, including while the
// renderer's catalog request is pending. Resolve the connection rather than
// splitting refs: routing model IDs can themselves contain slashes. Extension
// and composite controller labels remain owned by their runtime.
func desktopModelLabel(cfg *config.Config, ref, controllerLabel string) string {
	entry, ok := cfg.ResolveModel(ref)
	if !ok {
		return controllerLabel
	}
	if controllerLabel != "" && controllerLabel != entry.Model {
		return controllerLabel
	}
	info := configuredModelInfo(cfg, entry.Name, entry.Model, true)
	if name := strings.TrimSpace(info.ModelDisplayName); name != "" {
		return name
	}
	return entry.Model
}

func (a *App) recordModelSwitchTiming(tabID string, timing *modelSwitchTiming, started time.Time, result *error) {
	timing.Total = time.Since(started)
	if *result != nil {
		timing.Outcome = "failed"
	} else {
		timing.Outcome = "ok"
	}
	slog.Debug(
		"desktop: model switch timing",
		"tab", tabID,
		"outcome", timing.Outcome,
		"total_ms", timing.Total.Milliseconds(),
		"lock_wait_ms", timing.LockWait.Milliseconds(),
		"prepare_ms", timing.Prepare.Milliseconds(),
		"config_ms", timing.Config.Milliseconds(),
		"snapshot_ms", timing.Snapshot.Milliseconds(),
		"build_ms", timing.Build.Milliseconds(),
		"lease_resume_ms", timing.LeaseAndResume.Milliseconds(),
		"swap_persist_ms", timing.SwapAndPersist.Milliseconds(),
	)
	if a.modelSwitchTimingHook != nil {
		a.modelSwitchTimingHook(*timing)
	}
}

func (a *App) ModelsForTab(tabID string) []ModelInfo {
	a.mu.RLock()
	curModel := ""
	workspaceRoot := ""
	var ctrl control.SessionAPI
	if tab := a.tabByIDLocked(tabID); tab != nil {
		curModel = tab.model
		workspaceRoot = tab.WorkspaceRoot
		ctrl = tab.Ctrl
	}
	a.mu.RUnlock()
	return a.desktopModelCatalog(curModel, workspaceRoot, ctrl)
}

func (a *App) desktopModelCatalog(curModel, workspaceRoot string, ctrl control.SessionAPI) []ModelInfo {
	// A cold extension catalog fetch can block on a sidecar RPC, so read it
	// before loading config and without holding App.mu.
	var extensionCatalog []provider.Descriptor
	if ctrl != nil {
		extensionCatalog = ctrl.ProviderCatalog()
	}
	cfg, err := config.LoadForRoot(workspaceRoot)
	if err != nil {
		return []ModelInfo{}
	}
	if entry, ok := cfg.ResolveModel(curModel); ok {
		curModel = entry.Name + "/" + entry.Model
	}
	out := []ModelInfo{}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !modelProviderAccessAllowed(cfg.Desktop.ProviderAccess, p.Name) || !p.Configured() {
			continue
		}
		for _, m := range p.ChatModelList() {
			ref := p.Name + "/" + m
			out = append(out, configuredModelInfo(cfg, p.Name, m, ref == curModel))
		}
	}
	return mergeExtensionModelInfos(out, extensionCatalog, curModel)
}

// configuredModelInfo projects the same per-model overrides and capability
// resolver used by settings, without guessing capabilities for unknown models.
func configuredModelInfo(cfg *config.Config, name, model string, current bool) ModelInfo {
	info := ModelInfo{Ref: name + "/" + model, Provider: name, Model: model, Current: current}
	if entry, ok := cfg.ResolveModel(info.Ref); ok {
		if _, catalog, ok := config.CatalogForProviderEntry(entry); ok {
			info.BrandID = catalog.BrandID
		}
		info.DisplayName = entry.DisplayName
		info.ContextWindow = entry.ContextWindow
		capability := config.NewModelCapabilityResolver().Resolve(entry)
		info.ModelDisplayName = capability.ModelInfo.Name
		info.Vision = string(capability.State) == "supported"
	}
	return info
}
