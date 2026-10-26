package cli

import (
	"context"

	"deepzora/internal/control"
)

// prepareServeSession binds the resumed identity, if any, before serving.
func prepareServeSession(ctrl *control.Controller, target cliResumeTarget) error {
	if target.empty() {
		return nil
	}
	_, err := ctrl.OpenSessionFromService(context.Background(), resumeTargetService(target), target.ref)
	return err
}
