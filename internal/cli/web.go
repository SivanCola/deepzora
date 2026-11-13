package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	service "deepzora/desktop"
	"deepzora/internal/weboptions"
)

func webCommand(args []string, version string) int {
	// Parse here as well so --help works in a plain Go/source-only CLI build.
	options, err := weboptions.ParseOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deepzora web:", err)
		return 2
	}
	if options.Help {
		fmt.Fprint(os.Stdout, weboptions.Usage)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service.ConfigureBuildIdentity(version, "stable", "false")
	err = service.RunWeb(ctx, options)
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "deepzora web:", err)
	return 1
}
