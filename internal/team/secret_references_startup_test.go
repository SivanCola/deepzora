//go:build !windows

package team

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Exercise a process that takes longer than the old five-second startup limit.
// Native PowerShell's AST safety is covered separately on installed runtimes.
func TestPowerShellSecretParserAllowsColdProcessStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cold-parser")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 6\nprintf '%s' '{\"ok\":true,\"names\":[\"env:DEEPZORA_SECRET_DEPLOY\"]}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	names, err := ReferencedPowerShellSecrets(t.Context(), path, `$env:DEEPZORA_SECRET_DEPLOY`)
	if err != nil || !reflect.DeepEqual(names, []string{"DEPLOY"}) {
		t.Fatalf("cold parser names=%v err=%v", names, err)
	}
}
