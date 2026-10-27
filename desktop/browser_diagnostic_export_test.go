package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/desktop/internal/browserops"
)

type diagnosticRequestHost func(context.Context, string, any, any) error

func (f diagnosticRequestHost) Request(ctx context.Context, method string, params, result any) error {
	return f(ctx, method, params, result)
}

func TestBrowserDiagnosticCancellationRemainsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	entered := make(chan struct{})
	host := diagnosticRequestHost(func(ctx context.Context, _ string, _ any, _ any) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	done := make(chan any, 1)
	go func() { done <- captureBrowserHostDiagnostics(ctx, host, browserDiagnosticScope("host", "source")) }()
	<-entered
	cancel()
	data, _ := json.Marshal(<-done)
	if !bytes.Contains(data, []byte("capture_interrupted")) {
		t.Fatalf("lost cancellation evidence: %s", data)
	}
}

func TestBrowserDiagnosticHostCompatibilityAndIdentity(t *testing.T) {
	scope := browserDiagnosticScope("host", "session")
	for _, tc := range []struct {
		name      string
		reply     any
		err       error
		available bool
	}{
		{"supported", map[string]any{"available": true, "scope": scope, "entries": []any{}}, nil, true},
		{"old", nil, errors.New("method not found"), false},
		{"wrong-session", map[string]any{"available": true, "scope": "other"}, nil, false},
		{"oversized", map[string]any{"available": true, "scope": scope, "entries": strings.Repeat("x", 513<<10)}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := &fakeBrowserHost{replies: map[string]any{"host/browser.exportDiagnostics": tc.reply}, errs: map[string]error{"host/browser.exportDiagnostics": tc.err}}
			result := captureBrowserHostDiagnostics(t.Context(), host, scope)
			data, _ := json.Marshal(result)
			var parsed struct {
				Available bool `json:"available"`
			}
			if json.Unmarshal(data, &parsed) != nil || parsed.Available != tc.available {
				t.Fatalf("unexpected result %s", data)
			}
		})
	}
	if scope == browserDiagnosticScope("other-host", "session") || browserDiagnosticScope("host", "") != "" {
		t.Fatal("invalid scope isolation")
	}
}

func TestBrowserDiagnosticColdExportIncludesOnlyFixedSource(t *testing.T) {
	app, ref := activityBaselineFixture(t, "browser-diagnostic")
	ledger, err := app.browserLedger()
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []browserops.Operation{
		{ID: "source-operation", DiagnosticScope: browserDiagnosticScope(localDesktopHostID, ref.SessionID), Action: "click"},
		{ID: "foreign-operation", DiagnosticScope: browserDiagnosticScope(localDesktopHostID, "other"), Action: "click"},
	} {
		if err = ledger.Reserve(op); err != nil {
			t.Fatal(err)
		}
	}
	if err = app.desktopSessionService("").Close(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.json")
	app.setNativeHost(&recordingNativeHost{dialogPath: path, onCall: func(name string) {
		if strings.HasPrefix(name, "SaveFileDialog:") {
			app.activeTabID = "other"
		}
	}})
	handle, err := app.BeginSessionExportForTarget(SessionSelector{Ref: &ref}, "", "diagnostic", "Browser", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.FinishSessionExport(handle.ExportID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	section := result["browserDiagnostics"]
	var status struct {
		Host struct {
			Available bool `json:"available"`
		} `json:"host"`
	}
	if err = json.Unmarshal(section, &status); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(section, []byte("source-operation")) || bytes.Contains(section, []byte("foreign-operation")) || status.Host.Available {
		t.Fatalf("bad evidence %s", section)
	}
	if op, _ := ledger.Lookup("source-operation"); op.State != browserops.StateReserved {
		t.Fatal("export settled write")
	}
}
