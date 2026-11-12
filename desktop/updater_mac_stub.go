//go:build !darwin

package service

import (
	"fmt"
	"runtime"
)

func applyMac(string, string, int) error {
	return fmt.Errorf("self-update unsupported on %s", runtime.GOOS)
}

func maybeRunMacUpdateHandoff([]string) (bool, int) {
	return false, 0
}
