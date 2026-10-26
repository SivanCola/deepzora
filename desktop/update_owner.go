package main

import "os"

// updateOwnerPID is the process a platform update helper waits for before it
// swaps the install: the Electron shell, which launched this service and
// holds the bundle open.
func updateOwnerPID() int {
	return os.Getppid()
}
