package installsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"deepzora/internal/fileutil"
	"deepzora/internal/pluginpkg"
)

// One writer holds the market lock. Keep both trees on the same filesystem;
// the durable journal precedes either rename. A lost process can be recovered
// without consulting a branch, a network index, or plugin code.
type catalogTransaction struct {
	Previous *pluginpkg.InstalledPlugin `json:"previous,omitempty"`
	Next     pluginpkg.InstalledPlugin  `json:"next"`
}

func txnDir(home string) string {
	return filepath.Join(pluginpkg.PluginsDir(home), ".market-transaction")
}
func exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
func stageCatalog(home string, p pluginpkg.Package, src, target string, tx catalogTransaction) error {
	d := txnDir(home)
	if err := os.MkdirAll(filepath.Dir(d), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(d, 0700); err != nil {
		return fmt.Errorf("plugin transaction pending (run deepzora plugin recover): %w", err)
	}
	stage := filepath.Join(d, "next")
	if err := installCopiedPlugin(p, src, stage, false); err != nil {
		_ = os.RemoveAll(d)
		return err
	}
	b, err := json.Marshal(tx)
	if err == nil {
		err = fileutil.AtomicWriteFileStrict(filepath.Join(d, "journal.json"), b, 0600)
	}
	if err != nil {
		_ = os.RemoveAll(d)
		return err
	}
	rollback := func(cause error) error { return errors.Join(cause, recoverCatalog(home)) }
	if tx.Previous != nil {
		if err = os.Rename(target, filepath.Join(d, "previous")); err != nil {
			return rollback(err)
		}
	}
	if present, err := exists(target); err != nil || present {
		return rollback(errors.New("plugin target changed while installing"))
	}
	if err = os.Rename(stage, target); err != nil {
		return rollback(err)
	}
	if err = pluginpkg.Upsert(home, tx.Next); err != nil {
		return rollback(err)
	}
	return os.RemoveAll(d)
}
func RecoverCatalog(ctx context.Context, home string) error {
	t := &installSourceTool{deepzoraHome: home}
	release, err := t.lockCatalog(ctx)
	if err != nil {
		return err
	}
	defer release()
	return recoverCatalog(home)
}
func recoverCatalog(home string) error {
	d := txnDir(home)
	has, err := exists(d)
	if err != nil || !has {
		return err
	}
	var tx catalogTransaction
	b, err := os.ReadFile(filepath.Join(d, "journal.json"))
	if os.IsNotExist(err) {
		return os.RemoveAll(d)
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &tx); err != nil {
		return err
	}
	if !pluginpkg.IsValidName(tx.Next.Name) || !fullCommit.MatchString(tx.Next.Commit) || tx.Previous != nil && tx.Previous.Name != tx.Next.Name {
		return errors.New("invalid plugin recovery journal")
	}
	target := pluginpkg.InstallRoot(home, tx.Next.Name)
	stage := filepath.Join(d, "next")
	prior := filepath.Join(d, "previous")
	moved, err := exists(prior)
	if err != nil {
		return err
	}
	staged, err := exists(stage)
	if err != nil {
		return err
	}
	state, err := pluginpkg.LoadState(home)
	if err != nil {
		return err
	}
	var current *pluginpkg.InstalledPlugin
	for i := range state.Plugins {
		if state.Plugins[i].Name == tx.Next.Name {
			current = &state.Plugins[i]
			break
		}
	}
	if !staged && current != nil && *current == tx.Next {
		return os.RemoveAll(d)
	}
	if current != nil && (tx.Previous == nil || *current != *tx.Previous) || current == nil && tx.Previous != nil {
		return errors.New("installed record changed during recovery; previous package retained")
	}
	if !staged {
		if err = os.Rename(target, stage); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if moved {
		if err = os.Rename(prior, target); err != nil {
			return err
		}
	}
	return os.RemoveAll(d)
}
