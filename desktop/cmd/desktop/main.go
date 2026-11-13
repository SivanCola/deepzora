package main

import service "deepzora/desktop"

var (
	version       = "dev"
	channel       = "stable"
	macSelfUpdate = "false"
)

func main() {
	service.ConfigureBuildIdentity(version, channel, macSelfUpdate)
	service.RunDesktop()
}
