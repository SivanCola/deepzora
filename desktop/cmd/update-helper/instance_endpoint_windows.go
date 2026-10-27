//go:build windows

package main

import (
	"errors"
	"os"
	"unsafe"

	"deepzora/desktop/internal/instanceidentity"
	"golang.org/x/sys/windows"
)

var errEndpointStarting = errors.New("instance endpoint is busy without an identifiable owner")

var handoffUser32 = windows.NewLazySystemDLL("user32.dll")

// desktopEndpointImage resolves the executable that owns this data home's
// Electron service endpoint. A busy pipe means an owner exists but is between
// clients, so the caller may wait instead of treating the home as free.
func desktopEndpointImage(id string) (string, error) {
	image, err := instanceidentity.EndpointImage(id)
	if errors.Is(err, windows.ERROR_PIPE_BUSY) {
		return "", errEndpointStarting
	}
	return image, err
}

func notifyHandoffBlocked(installed bool) {
	// Background/silent update execution has no interactive recovery owner.
	// Its caller records the installed-versus-started result in the update log.
	if os.Getenv("DEEPZORA_INTERACTIVE_RECOVERY") != "1" {
		return
	}
	message := "Deepzora could not finish the update restart. Choose Quit in the old Deepzora window, then open Deepzora again. Your running instance was preserved."
	if installed {
		message = "The new version is installed, but restart is incomplete. Choose Quit in the old Deepzora window, then open Deepzora again. Your running instance was preserved."
	}
	message += "\n\n请在旧 Deepzora 窗口中选择退出，再重新打开 Deepzora。现有实例已保留。"
	title, _ := windows.UTF16PtrFromString("Deepzora update / 更新")
	body, _ := windows.UTF16PtrFromString(message)
	// The helper runs after the desktop has exited; log-only failures are invisible.
	handoffUser32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x00000030|0x00010000)
}
