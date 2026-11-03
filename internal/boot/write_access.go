package boot

import (
	"fmt"
	"io"
	"os"

	"deepzora/internal/config"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/hook"
	"deepzora/internal/lsp"
	"deepzora/internal/sandbox"
)

func newBootHookRunner(resolved []hook.ResolvedHook, root string, shell sandbox.Shell, sink event.Sink) *hook.Runner {
	if held, pending := hook.PendingProjectHooks(hook.LoadOptions{ProjectRoot: root}); pending {
		sink.Emit(projectHooksHeldEvent(held))
	}
	return newHookRunner(resolved, root, shell,
		func(msg string) { sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg}) })
}

// NewCommandHookRunner resolves hooks for a command that drives an agent
// without Build, from the sources load names; hooks run in load.ProjectRoot.
// Resolving shellCfg may execute its path, so it must come from a source the
// command trusts as much as the hooks themselves. Warnings go to warn.
func NewCommandHookRunner(shellCfg config.ShellConfig, load hook.LoadOptions, warn io.Writer) *hook.Runner {
	shell := sandbox.ResolveShell(shellCfg.Prefer, shellCfg.Path, warn)
	return newHookRunner(hook.Load(load), load.ProjectRoot, shell,
		func(msg string) { _, _ = fmt.Fprintln(warn, msg) })
}

func newHookRunner(resolved []hook.ResolvedHook, root string, shell sandbox.Shell, notify func(string)) *hook.Runner {
	runtime := hook.RuntimeOptions{}
	if shell.Kind == sandbox.ShellBash {
		runtime.BashPath = shell.Path
	}
	return hook.NewRunner(resolved, root, hook.NewDefaultSpawner(runtime), notify)
}

func projectWriteAccessPersister(root string) control.PersistWriteAccessFunc {
	return func(dirs []string, permRule string) error {
		return config.PersistWorkspaceWriteAccess(config.DeepzoraHomeDir(), root, dirs, permRule)
	}
}

func userHomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// projectHooksHeldEvent says which project hooks did not load and how to let them.
func projectHooksHeldEvent(p config.ProjectProgram) event.Event {
	return event.Event{
		Kind:   event.Notice,
		Level:  event.LevelWarn,
		Text:   "This project's hooks are off until you approve them.",
		Detail: fmt.Sprintf("%s declares: %s\nRun `deepzora trust` in this workspace to review and approve them; any later change needs approval again.", p.Name, p.Detail),
	}
}

// verifiedLSPSpecs checks a workspace-declared server against its approval
// before every start, so a file it names that changed mid-session is refused.
func verifiedLSPSpecs(cfg *config.Config) map[string]lsp.ServerSpec {
	specs := LSPSpecs(cfg.LSP)
	for lang, spec := range specs {
		spec.Verify = cfg.ProjectProgramVerifier(config.ProjectProgramLSP, lang)
		specs[lang] = spec
	}
	return specs
}
