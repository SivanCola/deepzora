//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"deepzora/desktop/internal/instanceidentity"
)

// This worker owns the same named-pipe endpoint the desktop service exposes.
func TestInstanceEndpointWorker(t *testing.T) {
	id := os.Getenv("DEEPZORA_TEST_ENDPOINT")
	if id == "" {
		t.Skip("subprocess fixture")
	}
	stop, err := instanceidentity.ListenEndpoint(id)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	marker := os.Getenv("DEEPZORA_TEST_MARKER")
	if err := os.WriteFile(marker, []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker + ".stop"); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fixture timed out")
}

func TestNativeEndpointIsolationAndHandoff(t *testing.T) {
	root, homeA, homeB := t.TempDir(), t.TempDir(), t.TempDir()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "deepzora-desktop.exe")
	old := filepath.Join(root, "versions", "old", "deepzora-desktop.exe")
	if err := os.MkdirAll(filepath.Dir(old), 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{target, old} {
		if err := os.WriteFile(p, payload, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DEEPZORA_HOME", homeA)
	t.Setenv(instanceidentity.UpdateEnvironmentKey, instanceidentity.ForHome(homeA))
	start := func(path, home string) func() {
		marker := filepath.Join(t.TempDir(), "ready")
		cmd := exec.Command(path, "-test.run=^TestInstanceEndpointWorker$")
		cmd.Env = append(os.Environ(), "DEEPZORA_TEST_ENDPOINT="+instanceidentity.ForHome(home), "DEEPZORA_TEST_MARKER="+marker)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			_ = os.WriteFile(marker+".stop", nil, 0600)
			if err := cmd.Wait(); err != nil {
				t.Errorf("worker: %v", err)
			}
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("worker did not register endpoint")
			}
			time.Sleep(20 * time.Millisecond)
		}
		return stop
	}
	start(old, homeB)
	if err := verifyDesktopHandoff(root, false); err != nil {
		t.Fatalf("independent old home blocked upgrade: %v", err)
	}
	stopA := start(old, homeA)
	if err := verifyDesktopHandoff(root, false); err == nil {
		t.Fatal("same-home old owner was accepted")
	}
	if image, err := desktopEndpointImage(instanceidentity.ForHome(homeB)); err != nil || image == "" {
		t.Fatalf("independent owner lost: %s %v", image, err)
	}
	stopA()
	start(target, homeA)
	if err := verifyDesktopHandoff(root, true); err != nil {
		t.Fatal(err)
	}
	if image, err := desktopEndpointImage(instanceidentity.ForHome(homeB)); err != nil || image == "" {
		t.Fatal(fmt.Errorf("independent owner did not survive: %w", err))
	}
}
