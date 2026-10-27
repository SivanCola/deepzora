// Command deepzora-launcher is the permanent thin desktop entry point for the
// versioned install layout. It reads InstallRoot/current.json, starts
// the active deepzora-desktop binary, and exits. It never counts crashes,
// chooses previous versions, or enters a product safe mode.
package main

import (
	"os"

	"deepzora/internal/desktoplauncher"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return desktoplauncher.Run(args, version)
}
