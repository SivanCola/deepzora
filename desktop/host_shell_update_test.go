package main

import (
	"os"
	"testing"
)

func TestUpdateOwnerIsTheShellParent(t *testing.T) {
	if got := updateOwnerPID(); got != os.Getppid() {
		t.Fatalf("update owner pid = %d, want shell parent %d", got, os.Getppid())
	}
}

func TestHostUpdateHandoffQuitsWithoutSchedulingASecondRestart(t *testing.T) {
	app, _, shell := newHostShellBridgeForTest(t, nil)
	app.relaunchDesktop(false)
	calls := shell.methods()
	if len(calls) != 1 || calls[0] != "host/app.quit" {
		t.Fatalf("helper-owned restart asked shell to %v", calls)
	}
	app.relaunchDesktop(true)
	calls = shell.methods()
	if len(calls) != 2 || calls[1] != "host/app.relaunch" {
		t.Fatalf("shell-owned restart asked shell to %v", calls)
	}
}
