package botchannels

import (
	"os"
	"runtime"

	"deepzora/internal/winaclresidue"
)

// RestrictCredentialFile makes a newly created, still-empty credential file
// private before any secrets are written. Windows requires a protected DACL;
// Chmod alone only changes its read-only attribute.
func RestrictCredentialFile(f *os.File) error {
	if runtime.GOOS == "windows" {
		return winaclresidue.ResetCredentialDACL(f.Name())
	}
	return f.Chmod(0600)
}
