//go:build darwin && !cgo

package service

import "errors"

var errDarwinWorkspaceWatchingRequiresCGO = errors.New("macOS workspace watching requires CGO and FSEvents")

func newWorkspaceWatcher() (workspaceWatcher, error) {
	return nil, errDarwinWorkspaceWatchingRequiresCGO
}
