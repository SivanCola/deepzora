package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"deepzora/desktop/internal/browserops"
	"deepzora/internal/config"
	"deepzora/internal/extension/rpcwire"
)

func browserDiagnosticScope(hostID, sessionID string) string {
	if hostID == "" || sessionID == "" {
		return ""
	}
	data, _ := json.Marshal([]string{hostID, sessionID})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (a *App) browserDiagnosticExport(job *sessionExportJob) map[string]any {
	out := map[string]any{
		"schemaVersion": 1, "capturedAt": time.Now().UTC(), "source": "desktop",
		"toolEvidence": "Browser tool calls, results and errors are in the session commits; their prefix cutoff can precede this runtime capture.",
		"coverage":     "Host/page events are a bounded in-memory window, lost on shell restart. This is not a complete browsing history. Legacy unattributed records are omitted.",
	}
	if job.browserScope == "" {
		out["unavailable"] = "No fixed browser diagnostic identity for this session."
		return out
	}
	a.browserExecMu.Lock()
	ledger := a.browserOps
	a.browserExecMu.Unlock()
	var operations browserops.DiagnosticSnapshot
	var err error
	if ledger != nil {
		operations = ledger.Diagnostics(job.browserScope)
	} else {
		operations, err = browserops.ReadDiagnostics(filepath.Join(config.MemoryUserDir(), "browser", "operations-v1.json"), job.browserScope)
	}
	if err != nil {
		out["operations"] = map[string]any{"available": false, "reason": "Operation ledger is absent, unreadable, unsupported or exceeds the read budget."}
	} else {
		out["operations"] = operations
	}
	var host hostRequester
	if a.hostShell != nil && a.hostShell.server != nil {
		host = a.hostShell.server
	}
	out["host"] = captureBrowserHostDiagnostics(job.ctx, host, job.browserScope)
	return out
}

func captureBrowserHostDiagnostics(ctx context.Context, host hostRequester, scope string) any {
	unavailable := func(kind, reason string) any {
		return map[string]any{"available": false, "kind": kind, "reason": reason}
	}
	if host == nil || scope == "" {
		return unavailable("host_unavailable", "Browser diagnostic host or fixed session identity is unavailable.")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var raw json.RawMessage
	if err := host.Request(ctx, "host/browser.exportDiagnostics", map[string]string{"scope": scope}, &raw); err != nil {
		if ctx.Err() != nil {
			return unavailable("capture_interrupted", "Browser diagnostic capture was cancelled or exceeded its three-second deadline.")
		}
		var response *rpcwire.ResponseError
		if errors.As(err, &response) && response.Code == -32601 {
			return unavailable("capability_unsupported", "This shell does not support browser diagnostic export.")
		}
		return unavailable("host_request_failed", "Browser diagnostic host failed or disconnected; no browser action was replayed.")
	}
	if len(raw) > 512<<10 || !json.Valid(raw) {
		return unavailable("invalid_response", "Invalid or oversized host diagnostic response.")
	}
	var header struct {
		Scope     string `json:"scope"`
		Available bool   `json:"available"`
	}
	if json.Unmarshal(raw, &header) != nil || header.Scope != scope || !header.Available {
		return unavailable("identity_unverified", "Host diagnostic response is unavailable or its session identity does not match.")
	}
	return raw
}
