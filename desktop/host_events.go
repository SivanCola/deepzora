package main

// hostEventNames lists every static event name this package emits through
// the runtime event bridge; the host contract digest covers it.
// host_events_test.go pins each entry to a string literal in the sources.
var hostEventNames = []string{
	"InboxChanged",
	"agent:event",
	"agent:ready",
	"config:load-warnings",
	"desktop:shell-status",
	"history-index:changed-v1",
	"project-tree:changed",
	"project-tree:changed-v2",
	"project-tree:runtime-changed",
	"runtime-state:changed",
	"runtime:rebuilt",
	"tab:meta",
	"terminal:exit",
	"terminal:output",
	"topic:activation",
	"updater:progress",
}
