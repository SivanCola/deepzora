//go:build !windows

package botchannels

import (
	"os"
	"testing"
)

func assertCredentialPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credential mode = %o, want 0600", info.Mode().Perm())
	}
}
