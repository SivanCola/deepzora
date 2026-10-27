package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"deepzora/internal/fileutil"
)

// ExportGoalDiagnostics lets the user save the authoritative event stream,
// including tool calls/results and the exact goal/runtime identity, without
// relying on the frontend's paginated transcript.
func (a *App) ExportGoalDiagnostics() (string, error) {
	tab, _ := a.activeTabAndCtrl()
	if tab == nil {
		return "", errors.New("goal diagnostics are unavailable for this session")
	}
	// Goal diagnostics share the session export owner.

	handle, err := a.BeginSessionExportForTarget(SessionSelector{}, tab.ID, "diagnostic", tab.TopicTitle, "")
	if err != nil || handle.ExportID == "" {
		return "", err
	}
	defer func() { _ = a.CancelSessionExport(handle.ExportID) }()
	result, err := a.FinishSessionExport(handle.ExportID)
	if err != nil {
		return "", err
	}
	if len(result.Paths) == 0 {
		return "", nil
	}
	return result.Paths[0], nil
}

func writeGoalDiagnosticsFile(path string, write func(io.Writer) error) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".deepzora-goal-diagnostics-*")
	if err != nil {
		return exportOperationError("save goal diagnostics", path, err)
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return exportOperationError("save goal diagnostics", path, err)
	}
	if err := write(tmp); err != nil {
		return exportOperationError("save goal diagnostics", path, err)
	}
	if err := tmp.Sync(); err != nil {
		return exportOperationError("save goal diagnostics", path, err)
	}
	if err := tmp.Close(); err != nil {
		return exportOperationError("save goal diagnostics", path, err)
	}
	if err := fileutil.ReplaceFile(tmpPath, path); err != nil {
		return exportOperationError("save goal diagnostics", path, err)
	}
	keep = true
	return nil
}
