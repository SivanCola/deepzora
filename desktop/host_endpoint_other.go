//go:build !windows

package service

func startUpdateEndpoint() (func(), error) { return func() {}, nil }
