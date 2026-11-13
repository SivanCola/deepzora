//go:build windows

package service

import (
	"os"

	"deepzora/internal/appidentity"
	"deepzora/internal/installlayout"
)

// applyDesktopProcessIdentity runs only for the native desktop executable.
// The package is also linked into the CLI, which must keep its own identity.
func applyDesktopProcessIdentity() {
	if err := appidentity.ApplyToCurrentProcess(); err != nil {
		println("Warning: apply Windows app identity:", err.Error())
	}
	executable, err := os.Executable()
	if err != nil {
		return
	}
	installRoot, err := installlayout.ResolveInstallRoot(executable)
	if err != nil {
		return
	}
	if err := appidentity.RepairOwnedShortcuts(installRoot); err != nil {
		println("Warning: repair Windows shortcut identity:", err.Error())
	}
}
