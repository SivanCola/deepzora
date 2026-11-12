// The native executable in cmd/desktop delegates here. The importable service
// also backs deepzora web; both use the same internal/boot and control engine.
package service

import (
	"fmt"
	"os"
	"strings"

	"deepzora/internal/skill/skillwatch"

	// Blank imports wire compile-time built-ins into their registries, exactly as
	// cmd/deepzora does — boot.Build resolves providers/tools from these registries.
	_ "deepzora/internal/provider/anthropic"
	_ "deepzora/internal/provider/openai"
	_ "deepzora/internal/provider/responses"
	_ "deepzora/internal/tool/builtin"
)

// Executable entry points pass their linked identity through ConfigureBuildIdentity.
var version = "dev"

// channel records the build's release line, injected via
// `-X main.channel=stable`. Only the stable release line is supported.
var channel = "stable"

// macSelfUpdate is injected as "true" only for Developer ID signed + notarized
// macOS release builds. Local/ad-hoc macOS builds keep the manual download path.
var macSelfUpdate = "false"

func macSelfUpdateAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(macSelfUpdate)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// RunDesktop is the Electron host entry. Web and Desktop share this package's
// application service; only their transports and native capabilities differ.
func RunDesktop() {
	// Contract generation is a build-time operation. Dispatch it before crash
	// capture so packaging cannot create files in the operator's Deepzora home.
	if dir, ok := emitContractDir(os.Args[1:]); ok {
		os.Exit(runEmitContract(dir))
	}
	// The shell asks for this before Chromium starts a child; it touches no
	// Deepzora state, so it runs ahead of crash capture and every other mode.
	if app, ok := stripPackageGrantsRequest(os.Args[1:]); ok {
		os.Exit(stripPackageGrants(os.Stdout, os.Stderr, app))
	}
	// Internal watcher-helper entry: the host-shared skill watch service
	// re-enters this executable so Windows directory watching never runs
	// in-process. Dispatch before any application initialization.
	if skillwatch.MaybeRunHelper() {
		return
	}
	// Older Windows builds could leave sandbox ACL residue behind after a
	// crash; sweep it in the background so startup never waits on icacls.
	// The detached macOS self-update child must run before any shell starts.
	if handled, exitCode := maybeRunMacUpdateHandoff(os.Args[1:]); handled {
		os.Exit(exitCode)
	}
	capturePreviousFatalCrash()
	installFatalCrashOutput()
	exitIfHostLaunchMode(os.Args[1:])
	if maybeRelaunchIfSuperseded() {
		return
	}
	exitIfShellBootstrapped(os.Args[1:])

	fmt.Fprintln(os.Stderr, "deepzora-desktop: no Electron desktop shell (app/) is installed beside this binary; reinstall Deepzora or run the packaged desktop app")
	os.Exit(1)
}
