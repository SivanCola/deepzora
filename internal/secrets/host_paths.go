package secrets

import (
	"deepzora/internal/pathidentity"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

var protectedHostPaths struct {
	sync.RWMutex
	paths []string
}

func RegisterHostSecretPath(path string) {
	if path == "" {
		return
	}
	resolved, err := pathidentity.Resolve(path, pathidentity.Options{FollowLeaf: true})
	if err != nil {
		return
	}
	protectedHostPaths.Lock()
	defer protectedHostPaths.Unlock()
	if slices.Contains(protectedHostPaths.paths, resolved.PhysicalPath) {
		return
	}
	protectedHostPaths.paths = append(protectedHostPaths.paths, resolved.PhysicalPath)
}

// HostSecretPaths snapshots host-owned namespaces for sandbox assembly. The
// in-process readers still check HostSecretPath on every access.
func HostSecretPaths() []string {
	protectedHostPaths.RLock()
	defer protectedHostPaths.RUnlock()
	return slices.Clone(protectedHostPaths.paths)
}
func HostSecretPath(path string) bool {
	protectedHostPaths.RLock()
	empty := len(protectedHostPaths.paths) == 0
	protectedHostPaths.RUnlock()
	if empty {
		return false
	}
	resolved, err := pathidentity.Resolve(path, pathidentity.Options{FollowLeaf: true})
	if err != nil {
		return false
	}
	protectedHostPaths.RLock()
	defer protectedHostPaths.RUnlock()
	for _, root := range protectedHostPaths.paths {
		rel, err := filepath.Rel(root, resolved.PhysicalPath)
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)) {
			return true
		}
	}
	return false
}

// HostSecretTree reports overlap with a protected namespace. Recursive readers
// must enforce protection in-process when an external walker cannot do so.
func HostSecretTree(path string) bool {
	resolved, err := pathidentity.Resolve(path, pathidentity.Options{FollowLeaf: true})
	if err != nil {
		return true
	}
	protectedHostPaths.RLock()
	defer protectedHostPaths.RUnlock()
	for _, root := range protectedHostPaths.paths {
		for _, pair := range [][2]string{{root, resolved.PhysicalPath}, {resolved.PhysicalPath, root}} {
			rel, err := filepath.Rel(pair[0], pair[1])
			if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)) {
				return true
			}
		}
	}
	return false
}
