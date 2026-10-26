package bootstrap

import (
	"os"
	"path/filepath"
	"strings"

	"deepzora/internal/store"
)

// LocalServe is a resident serve recorded in this machine's remote state
// directory. When this machine is the SSH target, the state files a remote
// desktop's bootstrap wrote over SFTP are local files here.
type LocalServe struct {
	Slug  string
	State ServeState
	Base  string // http://host:port of the loopback listener
	Token string
}

// DiscoverLocalServes lists the serves recorded under dir whose process alive
// reports as running and whose address and token are readable. Records of
// dead processes are skipped: a restarted desktop respawns its serve on a new
// port, and dialing the stale address only burns handshakes.
func DiscoverLocalServes(dir string, alive func(pid int) bool) []LocalServe {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []LocalServe
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "serve-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		state, err := UnmarshalState(data)
		if err != nil || state.PID <= 0 || !alive(state.PID) {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimPrefix(name, "serve-"), ".json")
		// The port file carries the address the running process bound; the
		// state's own address is the fallback.
		addr := state.Addr
		if port, err := os.ReadFile(filepath.Join(dir, store.RemoteServePortName(slug))); err == nil {
			if trimmed := strings.TrimSpace(string(port)); trimmed != "" {
				addr = trimmed
			}
		}
		if addr == "" {
			continue
		}
		token := ""
		if data, err := os.ReadFile(filepath.Join(dir, store.RemoteServeTokenName(slug))); err == nil {
			token = strings.TrimSpace(string(data))
		}
		if token == "" {
			continue
		}
		out = append(out, LocalServe{Slug: slug, State: state, Base: "http://" + addr, Token: token})
	}
	return out
}
