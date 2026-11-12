package service

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"deepzora/internal/filelock"
)

//go:embed all:frontend/dist
var webAssets embed.FS

// RunWeb exposes the shared application service through an authenticated
// loopback transport. Browser lifetimes never own the engine or session leases.
func RunWeb(ctx context.Context, o WebOptions) (runErr error) {
	assets, err := fs.Sub(webAssets, "frontend/dist")
	if err != nil {
		return err
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return errors.New("Web UI is missing; rebuild with make build")
	}
	desktopStateSubdir = "web-ui"
	if err := os.MkdirAll(desktopConfigDir(), 0o700); err != nil {
		return err
	}
	lockCtx, cancelLock := context.WithTimeout(ctx, 200*time.Millisecond)
	release, err := filelock.Acquire(lockCtx, filepath.Join(desktopConfigDir(), "service.lock"))
	cancelLock()
	if err != nil {
		return errors.New("another Web workspace is running for this Deepzora home; use its existing browser page or stop it first")
	}
	defer release()
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", o.Port))
	if err != nil {
		return err
	}
	defer listener.Close()
	token, err := randomHex(32)
	if err != nil {
		return err
	}
	service := NewService(ServiceOptions{Headless: true})
	app := service.app
	handler, err := newLocalWeb(service, assets, listener.Addr().String(), token)
	if err != nil {
		return err
	}
	if err := service.SetEventSink(handler.emit); err != nil {
		return err
	}
	// The signal only requests shutdown. The application context must outlive
	// it so the coordinated shutdown saves sessions before anything is cancelled.
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	if err := service.Start(appCtx); err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), hostDetachedShutdownTimeout)
		defer cancel()
		runErr = errors.Join(runErr, service.Shutdown(shutdownCtx))
	}()
	select {
	case <-app.tabsRestoredSignal():
	case <-ctx.Done():
		return nil
	}
	if _, err := app.SwitchWorkspace(o.Dir); err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	accessURL := "http://" + listener.Addr().String() + "/#token=" + token
	fmt.Fprintf(os.Stdout, "Deepzora Web: %s\nProject: %s\nPress Ctrl-C to stop.\n", accessURL, o.Dir)
	if !o.NoOpen {
		if err := openWebBrowser(accessURL); err != nil {
			fmt.Fprintln(os.Stderr, "Could not open the browser; open the URL above:", err)
		}
	}
	select {
	case <-ctx.Done():
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	handler.close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func openWebBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}
