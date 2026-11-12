package installsource

import (
	"context"
	"deepzora/internal/secrets"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"deepzora/internal/filelock"
	"deepzora/internal/plugincatalog"
	"deepzora/internal/pluginpkg"
)

var fullCommit = regexp.MustCompile(`^[a-f0-9]{40}$`)

type CatalogPlan struct {
	Execution            []string                     `json:"execution,omitempty"`
	PreviousExecution    []string                     `json:"previousExecution,omitempty"`
	Requirements         []pluginpkg.CapabilityRef    `json:"requirements,omitempty"`
	ID                   string                       `json:"id"`
	Channel              string                       `json:"channel"`
	Previous             *pluginpkg.InstalledPlugin   `json:"previous,omitempty"`
	PreviousCapabilities *pluginpkg.CapabilitySummary `json:"previousCapabilities,omitempty"`
	Capabilities         pluginpkg.CapabilitySummary  `json:"capabilities"`
}

func executionSummary(p pluginpkg.Package) []string {
	var lines []string
	for _, h := range p.Inventory().Hooks {
		if h.Command != "" {
			lines = append(lines, "hook "+h.Event+": "+secrets.RedactCredentials(h.Command))
		}
	}
	for n, m := range p.Manifest.MCPServers {
		lines = append(lines, "MCP "+n+": "+secrets.RedactCredentials(strings.TrimSpace(m.Command+" "+strings.Join(m.Args, " ")+" "+m.URL)))
	}
	if p.Manifest.Runtime != nil {
		lines = append(lines, "runtime: "+secrets.RedactCredentials(pluginpkg.RuntimeCommandLine(p.Manifest.Runtime)))
	}
	sort.Strings(lines)
	return lines
}
func catalogSpec(s string) bool { return plugincatalog.IsSpec(s) }
func (t *installSourceTool) lockCatalog(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(t.deepzoraHome, 0700); err != nil {
		return nil, err
	}
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return filelock.Acquire(c, filepath.Join(t.deepzoraHome, ".plugin-market.lock"))
}

// beginCatalogApply serializes catalog writes and settles any interrupted
// transaction before planning, so the plan is computed against settled state.
// Previews and non-catalog sources take no lock.
func (t *installSourceTool) beginCatalogApply(ctx context.Context, req request) (func(), error) {
	if !catalogSpec(req.Source) || !req.Apply {
		return func() {}, nil
	}
	if req.PlanID == "" {
		return nil, errors.New("catalog installation requires a preview and --plan-id")
	}
	unlock, err := t.lockCatalog(ctx)
	if err != nil {
		return nil, err
	}
	if err := recoverCatalog(t.deepzoraHome); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

func fetchSnapshot(ctx context.Context, tmp, url, sha string) error {
	for _, args := range [][]string{{"init", "--quiet", tmp}, {"-C", tmp, "fetch", "--no-tags", "--depth=1", url, sha}, {"-C", tmp, "checkout", "--quiet", "--detach", "FETCH_HEAD"}} {
		if out, err := pluginGitCommand(ctx, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("fetch pinned plugin %s: %w: %s", sha, err, out)
		}
	}
	out, err := pluginGitCommand(ctx, "-C", tmp, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(out)) != sha {
		return errors.New("plugin snapshot does not match approved commit")
	}
	return nil
}
func (t *installSourceTool) planCatalog(ctx context.Context, req request) ([]action, []string, error) {
	if req.Mode == "link" || req.Op != "install" || (req.Kind != "auto" && req.Kind != "plugin") {
		return nil, nil, errors.New("catalog entries require a plugin copy installation")
	}
	e, err := plugincatalog.Resolve(ctx, t.deepzoraHome, req.Source)
	if err != nil {
		return nil, nil, err
	}
	root, commit, cleanup, err := t.pluginSource(ctx, e.Source(), "copy")
	if err != nil {
		return nil, nil, err
	}
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	p, w, err := pluginpkg.ParseDir(root)
	if err != nil {
		return nil, w, err
	}
	if commit != e.Commit || p.Manifest.Name != e.ManifestName || p.Manifest.Version != e.Version || p.ManifestKind != e.ManifestKind {
		return nil, w, errors.New("catalog identity does not match the pinned manifest")
	}
	req.Mode = "copy"
	a, err := t.pluginPackageAction(req, p, e.Source())
	if err != nil {
		return nil, w, err
	}
	if !pluginpkg.IsValidName(a.Name) {
		return nil, w, errors.New("invalid install name")
	}
	c := &CatalogPlan{ID: strings.Split(e.Spec, "@")[0], Channel: e.Channel, Capabilities: p.CapabilitySummary()}
	c.Execution = executionSummary(p)
	c.Requirements = p.Manifest.Requires
	state, err := pluginpkg.LoadState(t.deepzoraHome)
	if err != nil {
		return nil, w, err
	}
	for _, old := range state.Plugins {
		if !strings.EqualFold(old.Name, a.Name) {
			continue
		}
		src, ok := parseGitHubRepoSource(old.Source)
		if old.Name != a.Name || !ok || !strings.EqualFold(src.Owner+"/"+src.Repo, e.Repo) || src.Path != e.Path || old.CatalogID != "" && (old.CatalogID != c.ID || old.Channel != e.Channel) {
			return nil, w, fmt.Errorf("%s belongs to another source; choose --name", a.Name)
		}
		if !req.Replace {
			return nil, w, errors.New("already installed; preview with --replace to update")
		}
		c.Previous = &old
		if prior, _, err := pluginpkg.ParseDir(pluginpkg.ResolveRoot(t.deepzoraHome, old.Root)); err == nil {
			s := prior.CapabilitySummary()
			c.PreviousCapabilities = &s
			c.PreviousExecution = executionSummary(prior)
		}
	}
	if c.Previous == nil {
		if _, err := os.Lstat(a.Target); err == nil {
			return nil, w, errors.New("target exists without a matching installed record; choose --name")
		} else if !os.IsNotExist(err) {
			return nil, w, err
		}
	}
	a.Catalog = c
	a.Commit = commit
	a.preparedRoot = root
	a.cleanup = cleanup
	keep = true
	return []action{a}, w, nil
}
func (t *installSourceTool) commitCatalogInstall(req request, a *action, p pluginpkg.Package, src, target, sha string) error {
	enabled := true
	if prev := a.Catalog.Previous; prev != nil {
		if !req.Replace {
			return errors.New("replacement not approved")
		}
		enabled = prev.Enabled
	}
	entry := pluginpkg.InstalledPlugin{Name: a.Name, Source: a.Source, Root: pluginpkg.RelativeRoot(t.deepzoraHome, target), Version: p.Manifest.Version, Description: p.Manifest.Description, ManifestKind: p.ManifestKind, Enabled: enabled, Commit: sha, CatalogID: a.Catalog.ID, Channel: a.Catalog.Channel}
	return stageCatalog(t.deepzoraHome, p, src, target, catalogTransaction{Previous: a.Catalog.Previous, Next: entry})
}
