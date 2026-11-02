# Agent presets

[简体中文](AGENT_PRESETS.zh-CN.md)

An agent preset saves a persona and an optional tool allowlist for a conversation.
Use one for a reviewer, documentation writer, or another recurring role. Model
selection, permission mode, and planning or verification policy remain separate
settings.

## Desktop

Open **Settings → Agent presets** to create a definition, edit it, copy it, or
delete it. Choose **User** to reuse it across projects or **Project** to save it
with the current workspace. The built-in **Default** definition is read-only;
copy it to start your own preset.

In an empty conversation, open the preset picker and choose **New agent preset…**
to go straight to the creation form. The icon selector also accepts PNG, JPEG
and WebP files up to 5 MB and 8192 × 8192 pixels. Adjust the square crop and zoom,
then choose **Use icon**. You can replace the image or restore the default icon.
The cropped 128 × 128 PNG is embedded in the definition, so copies keep their
icon even if the original image moves. Lists and the composer use the same image.

The icon panel offers a glyph grid, color palette and **Random** button. Colors
apply to built-in glyphs; uploaded images keep their original colors.
Choose one **Prompt mode**: extend the defaults, replace coding instructions,
or provide the complete system prompt. The body label and requirement follow
the selected mode. **Advanced settings** contains the runtime-context switch;
tool restrictions use a separate switch. These controls keep the existing YAML
prompt options and tool semantics.

New presets suggest an ID as you type the name, avoiding reserved IDs and clashes
in the selected scope. Once you edit the ID, name changes leave it alone. Latin
names use a lowercase slug; names without Latin letters or digits use a stable
`preset-…` ID. Saved IDs remain fixed, including when renaming a preset.

Choose a preset beside the model and permissions in an empty conversation's
composer. After the first turn starts, the selection is locked. Start another
conversation to choose a different preset. Saving a definition does not change a
running conversation's prompt or tools.

## Terminal

```sh
deepzora preset new reviewer --scope user --prompt "Review changes and explain findings." --tools read_file,grep,glob
deepzora preset list
deepzora preset show reviewer
deepzora --preset reviewer
deepzora run --preset reviewer "Review the changes in this workspace"
```

In chat, `/preset` or `/preset current` shows the current selection; `/preset list`
lists definitions. `/preset select reviewer` selects a role while the conversation
is empty; `/preset select default` restores the standard agent. See the
[CLI reference](CLI.md) for authoring commands and flags.

## Files and scopes

Definitions are Markdown files named `<id>.md`:

- User: `<Deepzora home>/presets/<id>.md`, normally `~/.deepzora/presets/`.
- Project: `<workspace>/.deepzora/presets/<id>.md`.

`DEEPZORA_HOME` also relocates user presets. A project definition overrides the
same ID in the user directory. IDs contain 1–64 lowercase letters, digits, or
hyphens and start with a letter or digit. `default` is reserved.

```markdown
---
name: Reviewer
description: Reviews code and reports actionable findings
keep-coding-instructions: true
tools: [read_file, grep, glob, "mcp:github"]
---
Review the requested changes. Explain each finding with a concrete failure
scenario and a file location. Ask before making edits.
```

Supported frontmatter fields:

| Field | Meaning |
| --- | --- |
| `name` | Display name; defaults to the file ID. |
| `description` | Description shown in the picker and management page. |
| `icon` | Optional display glyph for the Desktop list and composer picker: `bot` (default), `code`, `terminal`, `book-open`, `pen`, `search`, `bug`, `flask`, `shield`, `palette`, `rocket`, `brain`, `sparkles`, `database`, `globe`, or `school`. Unknown keys show the default glyph; changing the icon does not rebuild a session. |
| `icon-image` | Optional embedded square PNG data URL, at most 128 KiB and 128 × 128 pixels. Overrides `icon` for display; image load failures fall back to the glyph. Display-only and excluded from runtime fingerprints. Older versions that do not recognize this field reject the definition; update the host/CLI before sharing image presets. |
| `icon-color` | Optional six-digit hex color for built-in glyphs. Copied with the preset; excluded from runtime fingerprints. Older hosts/CLIs must be updated before sharing presets using this field. |
| `keep-coding-instructions` | Defaults to `true`: append the body to the base persona. `false` replaces that persona; a nonempty body is required. Core policies still apply unless `complete` is `true`. |
| `complete` | Defaults to `false`. `true` uses a nonempty body as the entire system prompt, suppressing core policies, standing documents, skill instructions, and v4 system text. Takes precedence over `keep-coding-instructions`. |
| `include-runtime-context` | Defaults to `true`. `false` reduces runtime context to the working directory (cwd) only, including in complete mode. |
| `tools` | Omit for all available tools. Use a list to admit only matching tools; `[]` admits no execution tools. |

Both new boolean fields are optional; existing files keep their previous prompt
behavior and runtime context. `complete: true` preserves the saved
`keep-coding-instructions` value so it applies again when complete mode is turned
off. Tool admission, permission checks, model selection, and execution policy
remain independent of these prompt options.

For a complete persona with only cwd runtime context:

```markdown
---
name: Translator
complete: true
include-runtime-context: false
tools: []
---
Translate the user's text into English. Return only the translation.
```

The CLI flags `--complete[=true|false]` and
`--include-runtime-context[=true|false]` work with `preset new`, `edit`, and
`copy`. Omitting a flag on edit/copy preserves its saved value. JSON definitions
use `complete` and `includeRuntimeContext`; older host responses that omit the
latter are decoded with runtime context enabled. Tool ceilings retain the
`null` (unrestricted) versus `[]` (no execution tools) distinction.

Unknown frontmatter keys and malformed definitions are rejected. Tool names may
use glob patterns such as `read_*`. The shell names `bash` and `pwsh` are treated
as portable aliases. Tool restrictions also apply to tools added later.

MCP admission is per server: `mcp:github` admits that server and `mcp:*` admits all
configured MCP servers. Per-tool entries such as `mcp__github__create_issue` are
rejected. `use_capability` remains available as a discovery and routing tool; it
does not grant access to tools or servers excluded by the preset. Existing
permission checks still govern admitted operations.

## Session behavior

The session records the selected ID. Reopening a saved custom-preset session
loads the current definition with that ID, including its latest persona and tool
allowlist. Forked history inherits the source preset. A running conversation
keeps its assembled system prefix and tool schemas stable across turns. Complete
mode changes newly assembled system text; it does not erase historical context,
such as earlier messages or summaries in resumed or forked conversations.

If a recorded definition is deleted, the session keeps the ID, reports a warning,
and runs the default agent until the file is restored. An explicit request to
start with an unknown ID fails. A malformed existing definition reports an error
so it can be corrected.
