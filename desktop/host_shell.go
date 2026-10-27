package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"

	"deepzora/desktop/internal/hostrpc"
)

// hostShellBridge is present only when the Electron shell drives this
// process. It replaces the in-process tray and the native quit hooks with
// host/* requests and routes the shell's hostEvent notifications back into
// the matching App entry points.
type hostShellBridge struct {
	app    *App
	server *hostrpc.Server
}

func (a *App) hostMode() bool { return a != nil && a.hostShell != nil }

type hostTrayLabels struct {
	OpenTitle   string `json:"openTitle"`
	OpenTooltip string `json:"openTooltip"`
	QuitTitle   string `json:"quitTitle"`
	QuitTooltip string `json:"quitTooltip"`
	Tooltip     string `json:"tooltip"`
}

type hostTrayResult struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason"`
}

func (b *hostShellBridge) trayLabels(locale string) hostTrayLabels {
	labels := trayMenuLabels(locale)
	return hostTrayLabels{
		OpenTitle:   labels.openTitle,
		OpenTooltip: labels.openTooltip,
		QuitTitle:   labels.quitTitle,
		QuitTooltip: labels.quitTooltip,
		Tooltip:     "Deepzora",
	}
}

func (b *hostShellBridge) startTray() bool {
	a := b.app
	if a.shuttingDown.Load() || a.forceQuit.Load() {
		return false
	}
	a.mu.Lock()
	if a.tray != nil {
		a.mu.Unlock()
		return true
	}
	t := newDesktopTray()
	a.tray = t
	a.desktopShell.trayState = "probing"
	a.desktopShell.trayReason = ""
	a.trayReady = false
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), rpcHostWindowTimeout)
	defer cancel()
	var result hostTrayResult
	err := b.server.Request(ctx, "host/tray.ensure", b.trayLabels(a.trayLocale()), &result)
	switch {
	case err != nil:
		a.setTrayHealth(t, "unavailable", "host_unreachable")
	case !result.Ready:
		reason := result.Reason
		if reason == "" {
			reason = "platform_no_tray"
		}
		a.setTrayHealth(t, "unavailable", reason)
	default:
		a.setTrayHealth(t, "ready", "")
	}
	if !result.Ready || err != nil {
		a.mu.Lock()
		if a.tray == t {
			a.tray = nil
		}
		a.mu.Unlock()
		return false
	}
	return true
}

func (b *hostShellBridge) stopTray() {
	a := b.app
	a.mu.Lock()
	t := a.tray
	a.tray = nil
	a.mu.Unlock()
	if t == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rpcHostWindowTimeout)
	defer cancel()
	_ = b.server.Request(ctx, "host/tray.destroy", struct{}{}, nil)
	a.setTrayHealth(nil, "unavailable", "tray_exited")
}

func (b *hostShellBridge) updateTrayLocale(locale string) {
	a := b.app
	a.mu.RLock()
	present := a.tray != nil
	a.mu.RUnlock()
	if !present {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rpcHostWindowTimeout)
	defer cancel()
	_ = b.server.Request(ctx, "host/tray.ensure", b.trayLabels(locale), nil)
}

// handleHostEvent maps shell-originated events (second instance, tray menu,
// app menu) onto their App entry points.
func (b *hostShellBridge) handleHostEvent(_ context.Context, name string, payload json.RawMessage) error {
	a := b.app
	switch name {
	case "secondInstance":
		a.goSafe("secondInstanceLaunch", a.secondInstanceLaunch)
	case "tray.open":
		a.goSafe("showFromTray", a.showFromTray)
	case "tray.quit":
		a.goSafe("quitFromTray", a.quitFromTray)
	case "menu.showWindow":
		a.goSafe("showMainWindow", a.showMainWindow)
	default:
		slog.Debug("desktop host: ignoring host event", "name", name)
	}
	return nil
}

// beforeClose translates the shell's close reason into the same signal the
// macOS Cmd+Q hook raised, so quit and updater exits bypass background close.
func (b *hostShellBridge) beforeClose(ctx context.Context, reason string) bool {
	if reason == "quit" || reason == "updater" {
		markSystemQuitRequested()
	}
	return b.app.beforeClose(ctx)
}

// relaunch asks the shell to restart the whole application; the shell then
// drives beforeClose and shutdown, so the caller must not exit on its own.
func (b *hostShellBridge) relaunch() error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcHostWindowTimeout)
	defer cancel()
	execPath := ""
	if runtime.GOOS != "darwin" {
		execPath = currentLauncherPath()
	}
	return b.server.Request(ctx, "host/app.relaunch", struct {
		Args     []string `json:"args"`
		ExecPath string   `json:"execPath,omitempty"`
	}{Args: []string{}, ExecPath: execPath}, nil)
}

// quit asks the shell to shut the application down without restarting it.
func (b *hostShellBridge) quit() error {
	ctx, cancel := context.WithTimeout(context.Background(), rpcHostWindowTimeout)
	defer cancel()
	return b.server.Request(ctx, "host/app.quit", struct{}{}, nil)
}

// relaunchDesktop asks the Electron shell to restart (or only quit, when an
// update helper owns the restart) after an update. The shell drives
// beforeClose and shutdown, so the service never exits on its own here.
func (a *App) relaunchDesktop(relaunchBinary bool) {
	if !a.hostMode() {
		slog.Warn("desktop host: relaunch requested before the shell connected")
		return
	}
	var err error
	if relaunchBinary {
		err = a.hostShell.relaunch()
	} else {
		err = a.hostShell.quit()
	}
	if err != nil {
		slog.Warn("desktop host: relaunch request failed", "err", err)
	}
}

// relaunchAfterPortableUpdate restarts Deepzora once the platform installer
// owns the swap. Under the shell on macOS the detached hand-off reopens the
// swapped bundle itself, so the shell must only quit.
func (a *App) relaunchAfterPortableUpdate() {
	if runtime.GOOS == "darwin" && a.hostMode() {
		if err := a.hostShell.quit(); err != nil {
			slog.Warn("desktop host: quit request failed", "err", err)
		}
		return
	}
	a.relaunchDesktop(runtime.GOOS == "linux")
}
