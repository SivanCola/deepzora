package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"deepzora/internal/config"
	"deepzora/internal/control"
	"deepzora/internal/i18n"
	"deepzora/internal/serve"
)

type serveFrontendOptions struct {
	address   string
	portFile  string
	tokenFile string
	pidFile   string
	// launchTokenPath names where the launch token is, so the
	// terminal shows a path rather than the secret.
	launchTokenPath string
}

type serveFrontendResources struct {
	listener    net.Listener
	displayAddr string
	artifacts   []string
}

func prepareServeFrontend(opts serveFrontendOptions) (_ *serveFrontendResources, err error) {
	resources := &serveFrontendResources{displayAddr: opts.address}
	defer func() {
		if err != nil {
			resources.release(true)
		}
	}()

	if opts.portFile != "" {
		resources.listener, err = net.Listen("tcp", opts.address)
		if err != nil {
			return nil, err
		}
		resources.displayAddr = resources.listener.Addr().String()
		if err = writeServeAddrFile(opts.portFile, resources.displayAddr); err != nil {
			return nil, err
		}
		resources.artifacts = append(resources.artifacts, opts.portFile)
	}
	if opts.pidFile != "" {
		if err = writeServePidFile(opts.pidFile); err != nil {
			return nil, err
		}
		resources.artifacts = append(resources.artifacts, opts.pidFile)
	}
	return resources, nil
}

func (r *serveFrontendResources) release(closeListener bool) {
	if closeListener && r.listener != nil {
		_ = r.listener.Close()
	}
	for _, path := range r.artifacts {
		_ = os.Remove(path)
	}
}

func runServeFrontend(ctrl *control.Controller, srv *serve.Server, cfg config.ServeConfig, opts serveFrontendOptions) int {
	resources, err := prepareServeFrontend(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	defer resources.release(false)
	srv.EnableProviderSetupForListener(resources.displayAddr)
	if srv.AuthMode() == "none" || srv.AuthMode() == "token" {
		if opts.launchTokenPath, err = launchTokenLocation(srv.AuthToken(), opts, resources); err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		}
	}
	reportServeFrontend(ctrl, srv, cfg, resources.displayAddr, opts)
	startServeBalanceDiagnostics(ctrl)
	return serveFrontendLoop(ctrl, srv, resources, opts)
}

func reportServeFrontend(ctrl *control.Controller, srv *serve.Server, cfg config.ServeConfig, address string, opts serveFrontendOptions) {
	fmt.Printf("deepzora serve — %s on http://%s\n", ctrl.Label(), address)
	reportServeAuth(os.Stdout, srv, address, opts)
	if warning := serve.PlainHTTPAuthWarning(cfg, address); warning != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", warning)
	}
}

// reportServeAuth names the file holding the launch token, never the token: a
// sandboxed command can read back a terminal multiplexer's scrollback.
func reportServeAuth(w io.Writer, srv *serve.Server, address string, opts serveFrontendOptions) {
	switch srv.AuthMode() {
	case "token":
		fmt.Fprintln(w, "  auth: token")
		fmt.Fprintf(w, "  share: http://%s/#token=<token> (token in %s)\n", address, opts.launchTokenPath)
	case "none":
		fmt.Fprintln(w, "  auth: none (changes and approvals still require the launch token)")
		fmt.Fprintf(w, "  approvals: http://%s/#token=<token> (token in %s)\n", address, opts.launchTokenPath)
	}
}

func startServeBalanceDiagnostics(ctrl *control.Controller) {
	go func() {
		if balance, err := ctrl.Balance(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "  balance: error — %v\n", err)
		} else if balance == nil {
			fmt.Fprintln(os.Stderr, "  balance: not configured (no balance_url for this provider)")
		} else {
			fmt.Printf("  balance: %s\n", balance.Display())
		}
	}()
}

func serveFrontendLoop(ctrl *control.Controller, srv *serve.Server, resources *serveFrontendResources, opts serveFrontendOptions) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if resources.listener == nil {
		if err := srv.RunGraceful(ctx, opts.address); err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		}
		return 0
	}
	if err := srv.RunGracefulListener(ctx, resources.listener); err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	return 0
}
