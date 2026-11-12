// Package plugincatalog reads the public market on explicit user requests only.
package plugincatalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"deepzora/internal/fileutil"
	"golang.org/x/mod/semver"
)

const base = "https://raw.githubusercontent.com/SivanCola/deepzora-plugins/dist/"
const maxSize = 4 << 20

// HostVersion is set once during process startup from the binary build identity.
var HostVersion string

var pair = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*/[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var segment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var sha = regexp.MustCompile(`^[a-f0-9]{40}$`)

type Release struct {
	MinDeepzora  string            `json:"minDeepzora,omitempty"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Repo         string            `json:"repo"`
	Path         string            `json:"path,omitempty"`
	Commit       string            `json:"commit"`
	Summary      map[string]string `json:"summary"`
	ManifestName string            `json:"manifestName"`
	ManifestKind string            `json:"manifestKind"`
	Platforms    []string          `json:"platforms,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Yanked       bool              `json:"yanked,omitempty"`
}
type Index struct {
	SchemaVersion int       `json:"schemaVersion"`
	Releases      []Release `json:"releases"`
	// Skipped counts malformed or conflicting rows Parse dropped.
	Skipped int `json:"-"`
}
type Candidate struct {
	Spec    string `json:"spec"`
	Version string `json:"version"`
}
type Entry struct {
	Upstream *Candidate `json:"upstream,omitempty"`
	Release
	Channel string `json:"channel"`
	Spec    string `json:"spec"`
}
type Result struct {
	Entries []Entry `json:"entries"`
	Warning string  `json:"warning,omitempty"`
}

func (r Release) Source() string {
	return strings.TrimSuffix("https://github.com/"+r.Repo+"/tree/"+r.Commit+"/"+r.Path, "/")
}
func (r Release) origin() string { return strings.ToLower(r.Repo) + "/" + r.Path }
func (r Release) Compatible() bool {
	if r.MinDeepzora != "" {
		host := "v" + strings.TrimPrefix(HostVersion, "v")
		if !semver.IsValid(host) || semver.Compare(host, "v"+r.MinDeepzora) < 0 {
			return false
		}
	}
	if len(r.Platforms) == 0 {
		return true
	}
	return slices.Contains(r.Platforms, runtime.GOOS)
}

// Parse keeps every well-formed release and drops the rest. The unreviewed
// index is assembled from repositories anyone can tag, so one malformed row
// must not hide the others; a dropped row cannot be resolved, so installation
// still fails closed for it.
func Parse(b []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return idx, err
	}
	if idx.SchemaVersion != 1 || idx.Releases == nil {
		return idx, errors.New("unsupported or incomplete plugin catalog")
	}
	seen := map[string]bool{}
	sources := map[string]string{}
	ids := map[string]string{}
	kept := make([]Release, 0, len(idx.Releases))
	for _, r := range idx.Releases {
		key := r.ID + "@" + r.Version
		if validRelease(r) != nil || seen[key] || sources[r.origin()] != "" && sources[r.origin()] != r.ID || ids[r.ID] != "" && ids[r.ID] != r.origin() {
			idx.Skipped++
			continue
		}
		seen[key] = true
		sources[r.origin()] = r.ID
		ids[r.ID] = r.origin()
		kept = append(kept, r)
	}
	idx.Releases = kept
	sort.Slice(idx.Releases, func(i, j int) bool {
		a, b := idx.Releases[i], idx.Releases[j]
		if a.ID == b.ID {
			return semver.Compare("v"+a.Version, "v"+b.Version) > 0
		}
		return a.ID < b.ID
	})
	return idx, nil
}

func validRelease(r Release) error {
	if !pair.MatchString(r.ID) || strings.ToLower(r.ID) != r.ID || !pair.MatchString(r.Repo) || !sha.MatchString(r.Commit) || !segment.MatchString(r.ManifestName) || strings.TrimSpace(r.Name) == "" || !semver.IsValid("v"+r.Version) || semver.Canonical("v"+r.Version) != "v"+r.Version {
		return fmt.Errorf("invalid release %q @ %q", r.ID, r.Version)
	}
	if strings.TrimSpace(r.Summary["en"]) == "" || strings.TrimSpace(r.Summary["zh"]) == "" {
		return fmt.Errorf("missing localized summary for %s", r.ID)
	}
	if r.ManifestKind != "deepzora" && r.ManifestKind != "codex" && r.ManifestKind != "claude" {
		return fmt.Errorf("unsupported manifest kind %s", r.ManifestKind)
	}
	if r.Path != "" {
		if path.Clean(r.Path) != r.Path {
			return errors.New("invalid package path")
		}
		for s := range strings.SplitSeq(r.Path, "/") {
			if !segment.MatchString(s) {
				return errors.New("invalid package path")
			}
		}
	}
	if r.MinDeepzora != "" && !semver.IsValid("v"+r.MinDeepzora) {
		return fmt.Errorf("invalid minDeepzora for %s", r.ID)
	}
	return nil
}
func location(topic bool) (string, string) {
	key, file, channel := "DEEPZORA_PLUGIN_CATALOG", "index-v1.json", "community"
	if topic {
		key, file, channel = "DEEPZORA_PLUGIN_TAGGED", "tagged.json", "unreviewed"
	}
	if v := os.Getenv(key); v != "" {
		return v, "custom"
	}
	return base + file, channel
}

type cachedIndex struct {
	ETag string          `json:"etag,omitempty"`
	Body json.RawMessage `json:"body"`
}

func fetch(ctx context.Context, loc string, prior cachedIndex) (cachedIndex, error) {
	var rd io.ReadCloser
	var tag string
	if strings.HasPrefix(loc, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
		if err != nil {
			return cachedIndex{}, err
		}
		if prior.ETag != "" {
			req.Header.Set("If-None-Match", prior.ETag)
		}
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return cachedIndex{}, err
		}
		rd = resp.Body
		defer rd.Close()
		if resp.StatusCode == http.StatusNotModified && prior.ETag != "" {
			return prior, nil
		}
		if resp.StatusCode != http.StatusOK {
			return cachedIndex{}, fmt.Errorf("catalog: HTTP %s", resp.Status)
		}
		tag = resp.Header.Get("ETag")
	} else {
		if !filepath.IsAbs(loc) {
			return cachedIndex{}, errors.New("catalog must be HTTPS or an absolute local filename")
		}
		f, err := os.Open(loc)
		if err != nil {
			return cachedIndex{}, err
		}
		rd = f
		defer rd.Close()
	}
	b, err := io.ReadAll(io.LimitReader(rd, maxSize+1))
	if len(b) > maxSize {
		return cachedIndex{}, errors.New("catalog too large")
	}
	return cachedIndex{ETag: tag, Body: b}, err
}

// Browse may use the last good snapshot. Install requires a successful fetch
// or conditional revalidation from the selected authority.
func read(ctx context.Context, home string, topic, fresh bool) (Index, string, string, error) {
	loc, channel := location(topic)
	f := filepath.Join(home, "cache", "plugin-market", fmt.Sprintf("%x.json", sha256.Sum256([]byte(loc))))
	var prior cachedIndex
	var cached Index
	if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &prior) == nil {
		if i, e := Parse(prior.Body); e == nil {
			cached = i
		} else {
			prior = cachedIndex{}
		}
	}
	if !fresh && cached.Releases != nil {
		ttl := 6 * time.Hour
		if topic {
			ttl = 10 * time.Minute
		}
		if st, err := os.Stat(f); err == nil && time.Since(st.ModTime()) < ttl {
			return cached, channel, "", nil
		}
	}
	next, err := fetch(ctx, loc, prior)
	if err == nil {
		var i Index
		i, err = Parse(next.Body)
		if err == nil {
			if b, e := json.Marshal(next); e == nil && os.MkdirAll(filepath.Dir(f), 0700) == nil {
				_ = fileutil.AtomicWriteFile(f, b, 0600)
			}
			return i, channel, "", nil
		}
	}
	if !fresh && cached.Releases != nil {
		return cached, channel, "offline cache: " + err.Error(), nil
	}
	return Index{}, channel, "", err
}
func Search(ctx context.Context, home, q string, topics bool) (Result, error) {
	out := Result{Entries: []Entry{}}
	origins := map[string]bool{}
	listed := map[string]int{}
	for _, topic := range []bool{false, true} {
		if topic && !topics {
			break
		}
		idx, ch, w, err := read(ctx, home, topic, false)
		if err != nil {
			if !topic {
				return out, err
			}
			out.Warning = err.Error()
			break
		}
		if w != "" {
			out.Warning = w
		} else if idx.Skipped > 0 {
			out.Warning = fmt.Sprintf("%d malformed catalog entries were ignored", idx.Skipped)
		}
		for _, r := range idx.Releases {
			if r.Yanked || !r.Compatible() || semver.Prerelease("v"+r.Version) != "" {
				continue
			}
			if origins[r.origin()] {
				if i, ok := listed[r.origin()]; topic && ok && out.Entries[i].Upstream == nil && semver.Compare("v"+r.Version, "v"+out.Entries[i].Version) > 0 {
					out.Entries[i].Upstream = &Candidate{"topic:" + r.ID + "@" + r.Version, r.Version}
				}
				continue
			}
			origins[r.origin()] = true
			if !strings.Contains(strings.ToLower(strings.Join([]string{r.ID, r.Name, r.Repo, r.Summary["en"], r.Summary["zh"], strings.Join(r.Capabilities, " ")}, " ")), strings.ToLower(q)) {
				continue
			}
			prefix := "catalog:"
			if topic {
				prefix = "topic:"
			}
			if !topic {
				listed[r.origin()] = len(out.Entries)
			}
			out.Entries = append(out.Entries, Entry{Release: r, Channel: ch, Spec: prefix + r.ID + "@" + r.Version})
		}
	}
	return out, nil
}
func IsSpec(s string) bool { return strings.HasPrefix(s, "catalog:") || strings.HasPrefix(s, "topic:") }
func Resolve(ctx context.Context, home, s string) (Entry, error) {
	prefix, rest, _ := strings.Cut(s, ":")
	id, v, _ := strings.Cut(rest, "@")
	idx, ch, _, err := read(ctx, home, prefix == "topic", true)
	if err != nil {
		return Entry{}, err
	}
	for _, r := range idx.Releases {
		if r.ID == id && !r.Yanked && r.Compatible() && (r.Version == v || v == "" && semver.Prerelease("v"+r.Version) == "") {
			return Entry{Release: r, Channel: ch, Spec: prefix + ":" + id + "@" + r.Version}, nil
		}
	}
	return Entry{}, fmt.Errorf("no installable release for %s (absent, withdrawn, or incompatible)", s)
}
