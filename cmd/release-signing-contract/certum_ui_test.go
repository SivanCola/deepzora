package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPowerShellCertumWindowReadiness(t *testing.T) {
	script, err := filepath.Abs("../../scripts/certum-login-ui.test.ps1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range []string{"pwsh", "powershell.exe"} {
		binary, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		found = true
		t.Run(name, func(t *testing.T) {
			scripts := []string{script}
			if runtime.GOOS == "windows" {
				for _, name := range []string{"test-windows-installer-startup.test.ps1", "test-windows-upgrade-startup.test.ps1"} {
					scripts = append(scripts, filepath.Join(filepath.Dir(script), name))
				}
			}
			for _, file := range scripts {
				command := exec.Command(binary, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", file)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("Windows signing/acceptance regression %s: %v\n%s", file, err, output)
				}
			}
		})
	}
	if !found {
		t.Skip("PowerShell is unavailable; Windows CI runs both PowerShell runtimes")
	}
}

func TestCertumOfficialClientReadiness(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Skip("Official client probe requires a disposable GitHub-hosted Windows runner")
	}
	binary, err := exec.LookPath("pwsh")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// The probe has no signing credentials and never submits a login. It checks
	// that the official pinned client exposes the control roles our wait needs.
	command := exec.Command(binary, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", filepath.Join(root, "scripts/certum-client-probe.ps1"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Official SimplySign form probe: %v\n%s", err, output)
	}
}
