package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"deepzora/desktop/internal/hostrpc"
	"deepzora/internal/control"
	"deepzora/internal/session"
)

func (a *App) writeSessionDiagnosticExport(job *sessionExportJob) error {
	browserDiagnostics := a.browserDiagnosticExport(job)
	extra := map[string]any{"sessionIdentity": map[string]any{"session": job.handle.Snapshot.Ref, "source": "local", "workspaceRoot": job.workspaceRoot, "storageGeneration": job.handle.Snapshot.StorageGeneration}, "exportSnapshot": job.handle.Snapshot, "frontendObservation": job.observation}
	var frontend map[string]json.RawMessage
	_ = json.Unmarshal(job.observation, &frontend)
	if value := frontend["readDiagnostics"]; len(value) > 0 {
		extra["readDiagnostics"] = value
	}
	if len(job.observation) == 0 {
		extra["frontendObservation"] = nil
	}
	return writeGoalDiagnosticsFile(job.path, func(dst io.Writer) error {
		extra["browserDiagnostics"] = browserDiagnostics
		metadata := control.GoalDiagnosticMetadata{ApplicationVersion: version, BuildCommit: buildCommit(), ProtocolVersion: hostrpc.ProtocolVersion, Capabilities: []string{"session-export-v1", "goal-lifecycle-v2"}}
		if a.sessionDiagnosticControllerCurrent(job.controller, job.handle.Snapshot.Ref) {
			runtimeErr := job.controller.WriteSessionDiagnostics(job.ctx, dst, metadata, extra)
			if a.sessionDiagnosticControllerCurrent(job.controller, job.handle.Snapshot.Ref) {
				return runtimeErr
			}
			file, ok := dst.(*os.File)
			if !ok {
				return errors.Join(runtimeErr, errors.New("session controller changed during diagnostic capture"))
			}
			if resetErr := file.Truncate(0); resetErr != nil {
				return errors.Join(runtimeErr, resetErr)
			}
			if _, resetErr := file.Seek(0, io.SeekStart); resetErr != nil {
				return errors.Join(runtimeErr, resetErr)
			}
		}
		return control.WriteColdSessionDiagnostics(job.ctx, dst, job.query, job.handle.Snapshot, metadata, extra)
	})
}

func (a *App) sessionDiagnosticControllerCurrent(controller *control.Controller, ref session.SessionRef) bool {
	if controller == nil {
		return false
	}
	boundRef, bound := controller.SessionRef()
	if !bound || boundRef != ref {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.tabs {
		if !tab.removed && tab.Ctrl == controller && tab.SessionID == ref.SessionID {
			return true
		}
	}
	return false
}
