package main

import (
	"fmt"

	"deepzora/internal/control"
)

type toolRecoveryController interface {
	ToolRecoverySnapshot() control.ToolRecoverySnapshot
}

// Both local and remote surfaces use the same generation-bound request shape.
func (a *App) GetToolRecoveryForTab(tabID string) (control.ToolRecoverySnapshot, error) {
	_, ctrl := a.tabAndCtrlByID(tabID)
	if target, ok := ctrl.(toolRecoveryController); ok {
		return target.ToolRecoverySnapshot(), nil
	}
	return control.ToolRecoverySnapshot{}, fmt.Errorf("tool recovery unavailable")
}
