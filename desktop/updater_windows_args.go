package main

import (
	"strconv"
)

func windowsVersionedUpdateHandoffArgs(parentPID int, installer, installerSHA256, installDir, relaunch, toVersion string) []string {
	args := []string{
		"--parent-pid", strconv.Itoa(parentPID),
		"--installer", installer,
		"--installer-sha256", installerSHA256,
		"--to-version", toVersion,
		"--install-layout", "versioned-v1",
	}
	if installDir != "" {
		args = append(args, "--install-dir", installDir)
	}
	if relaunch != "" {
		args = append(args, "--relaunch", relaunch)
	}
	return args
}
