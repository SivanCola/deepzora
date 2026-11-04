package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/config"
	"deepzora/internal/control"
)

type BotModelView struct {
	Ref           string `json:"ref"`
	Provider      string `json:"provider"`
	ProviderLabel string `json:"providerLabel"`
	Model         string `json:"model"`
	Configured    bool   `json:"configured"`
}
type BotWorkspaceView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Root  string `json:"root"`
}

func botModelAvailable(cfg *config.Config, ref string) (*config.ProviderEntry, error) {
	p, ok := cfg.ResolveModel(ref)
	if !ok || p.Name+"/"+p.Model != ref || !slices.Contains(p.ChatModelList(), p.Model) ||
		(cfg.Desktop.ProviderAccess != nil && !slices.Contains(cfg.Desktop.ProviderAccess, p.Name)) {
		return nil, errors.New("bot model is unavailable; choose an added chat model in Model Services")
	}
	if !p.Configured() {
		return nil, errors.New("bot model has no API key; configure its connection in Model Services")
	}
	return p, nil
}

func (a *App) botModelCatalog() ([]BotModelView, error) {
	cfg, _, err := a.loadDesktopUserConfigForView()
	if err != nil {
		return nil, err
	}
	models := []BotModelView{}
	for _, p := range cfg.Providers {
		if cfg.Desktop.ProviderAccess != nil && !slices.Contains(cfg.Desktop.ProviderAccess, p.Name) {
			continue
		}
		label := p.DisplayName
		if label == "" {
			label = p.Name
		}
		configured := p.Configured()
		for _, m := range p.ChatModelList() {
			models = append(models, BotModelView{Ref: p.Name + "/" + m, Provider: p.Name, ProviderLabel: label, Model: m, Configured: configured})
		}
	}
	return models, nil
}

func botDedicatedOperation(c botchannels.Config) string {
	sum := sha256.Sum256([]byte(c.ID + "\x00" + c.WorkspaceID + "\x00" + c.ModelRef))
	return fmt.Sprintf("bot-session-%x", sum[:16])
}
func botDedicatedSession(c botchannels.Config) string {
	sum := sha256.Sum256([]byte(botDedicatedOperation(c)))
	return fmt.Sprintf("desktop-bot-%x", sum[:16])
}

func (a *App) validateBotModel(c botchannels.Config) error {
	cfg, _, err := a.loadDesktopUserConfigForView()
	if err != nil {
		return err
	}
	if _, err := botModelAvailable(cfg, c.ModelRef); err != nil {
		return err
	}
	state, err := a.workspaceRegistry().Load(a.bootContext())
	if err != nil {
		return err
	}
	if c.WorkspaceID != "global" {
		if w, ok := state.Workspaces[c.WorkspaceID]; !ok || !w.Visible {
			return errors.New("bot workspace is unavailable; choose an existing project")
		}
	}
	return nil
}

// Explicit user action; reuses the existing bounded model probe and stored
// credentials. No model API key is accepted or returned by Bot configuration.
func (a *App) TestBotModel(ref string) error {
	cfg, _, err := a.loadDesktopUserConfigForView()
	if err != nil {
		return err
	}
	p, err := botModelAvailable(cfg, ref)
	if err != nil {
		return err
	}
	if err := a.TestProviderModel(providerViewFromEntry(*p, false, true), p.Model, ""); err != nil {
		// Provider errors can echo secrets. Keep their raw body out of this API.
		return errors.New("model test failed; check the API key, model access and network in Model Services")
	}
	return nil
}

func (a *App) prepareDedicatedBotSession(ctx context.Context, c botchannels.Config) error {
	if c.SessionID != botDedicatedSession(c) {
		return errors.New("invalid dedicated session identity; save the bot again")
	}
	if err := a.validateBotModel(c); err != nil {
		return err
	}
	// Ensure the standard global workspace exists before using its registry ID.
	if c.WorkspaceID == "global" {
		if _, err := a.ensureDesktopWorkspace(ctx, "global", ""); err != nil {
			return err
		}
	}
	v, err := a.beginManualSessionCreation(ManualSessionCreationRequest{OperationID: botDedicatedOperation(c), WorkspaceID: c.WorkspaceID}, c.ModelRef)
	if err != nil {
		return err
	}
	if v.Phase == "failed" {
		v, err = a.RetryManualSessionCreation(v.OperationID)
		if err != nil {
			return err
		}
		// Retry is asynchronous; its persisted phase can remain failed briefly.
		v.Phase = "starting"
	}
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for v.Phase != "ready" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		v, err = a.GetManualSessionCreation(v.OperationID)
		if err != nil {
			return err
		}
		if v.Progress != nil && v.Progress.Status == "blocked" {
			return errors.New("dedicated session creation is blocked; inspect Desktop diagnostics before retrying")
		}
		if v.Phase == "failed" && (v.Progress == nil || v.Progress.Status == "completed" || v.Progress.Status == "blocked") {
			return errors.New("dedicated session could not start; check its model and workspace in Desktop")
		}
	}
	// A closed tab can be reopened from the same durable creation without
	// selecting it or replacing the user's active conversation.
	stored, err := a.desktopSessionService("").Query().OpenSession(ctx, v.Ref)
	if err != nil {
		return err
	}
	if stored.Recent.ModelRef != c.ModelRef {
		return errors.New("dedicated session model changed in Desktop; restore its configured model or choose a new model for this bot")
	}
	a.mu.RLock()
	live := a.liveRuntimeTabMatchingLocked(nil, sessionRoute(c.SessionID))
	a.mu.RUnlock()
	if live == nil {
		if err := a.createManualSessionRuntime(ctx, v, func(string) {}); err != nil {
			return err
		}
	}
	if v.Ref.SessionID != c.SessionID {
		return errors.New("dedicated session identity mismatch")
	}
	if meta := a.metaForSession(c.SessionID); meta != nil && meta.TopicTitle == defaultTopicTitle {
		if err := a.RenameCanonicalSession(v.Ref, "Bot · "+c.Name); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) bindBotRun(r *botChannelRun) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, t := range a.runtimeTabsLocked() {
		if r.config.Mode != "dedicated" && a.tabs[t.ID] != t {
			continue
		}
		if t.SessionID != r.config.SessionID || !t.Ready || t.ReadOnly {
			continue
		}
		c, ok := t.Ctrl.(*control.Controller)
		if !ok {
			continue
		}
		if r.config.Mode == "dedicated" && c.ModelRef() != r.config.ModelRef {
			return errors.New("dedicated session model changed in Desktop; restore its configured model or choose a new model for this bot")
		}
		if auth := c.AuthenticationState(); !auth.Ready() {
			return errors.New("model authentication is not ready; update the model credential in Model Services before connecting")
		}
		r.mu.Lock()
		r.ctrl, r.sink, r.tabID, r.generation = c, t.sink, t.ID, t.SessionGeneration
		r.mu.Unlock()
		return nil
	}
	return errBotSessionUnavailable
}

func botModelFailure(c *control.Controller) string {
	if c == nil {
		return ""
	}
	s := c.AuthenticationState()
	if s.Ready() {
		return ""
	}
	if s.Status == control.AuthenticationRejected {
		return "Model authentication failed; update the API key in Model Services."
	}
	return "Model credential unavailable; check Model Services."
}

func isDedicatedBotSession(id string) bool { return strings.HasPrefix(id, "desktop-bot-") }
