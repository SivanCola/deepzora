package team

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"deepzora/internal/proc"
	"deepzora/internal/teamroster"
)

// ReferencedPOSIXSecrets only recognizes direct shell AST expansions. A single
// quoted literal, indirect name, invalid script or non-POSIX dialect grants no
// secret environment. Parsing never evaluates the command.
func ReferencedPOSIXSecrets(command string) ([]string, error) {
	if len(command) > 128<<10 {
		return nil, errors.New("secret command exceeds parsing budget")
	}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.New("cannot parse secret command; use a direct $DEEPZORA_SECRET_NAME reference in a valid POSIX command")
	}
	names := []string{}
	syntax.Walk(file, func(node syntax.Node) bool {
		param, ok := node.(*syntax.ParamExp)
		if !ok || param.Param == nil {
			return true
		}
		if param.Excl || param.Index != nil || param.Slice != nil || param.Repl != nil || param.Exp != nil || param.Length || param.Width {
			return true
		}
		if name, ok := strings.CutPrefix(param.Param.Value, teamroster.SecretEnvPrefix); ok && teamroster.NormalizeSecretName(name) == name && name != "" {
			names = append(names, name)
		}
		return true
	})
	slices.Sort(names)
	return slices.Compact(names), nil
}

// The parser receives source through stdin as data. It never invokes the AST,
// ScriptBlock.Create, Invoke-Expression, dot-sourcing or the supplied command.
const powershellSecretParser = `$ErrorActionPreference='Stop'
$source=[Console]::In.ReadToEnd()
$tokens=$null;$parseErrors=$null
$ast=[System.Management.Automation.Language.Parser]::ParseInput($source,[ref]$tokens,[ref]$parseErrors)
if($parseErrors.Count -gt 0){[Console]::Out.Write('{"ok":false,"names":[]}');exit 0}
$names=@($ast.FindAll({param($n) $n -is [System.Management.Automation.Language.VariableExpressionAst]},$true) | ForEach-Object { $_.VariablePath.UserPath } | Where-Object { $_ -cmatch '^env:DEEPZORA_SECRET_[A-Z][A-Z0-9_]{0,39}$' } | Sort-Object -Unique)
[Console]::Out.Write((@{ok=$true;names=@($names)} | ConvertTo-Json -Compress))`

func ReferencedPowerShellSecrets(ctx context.Context, executable, command string) ([]string, error) {
	if len(command) > 128<<10 {
		return nil, errors.New("secret command exceeds parsing budget")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := proc.CommandContext(ctx, executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", powershellSecretParser)
	cmd.Stdin = strings.NewReader(command)
	var out boundedSecretParserOutput
	cmd.Stdout = &out
	cmd.Stderr = &boundedSecretParserOutput{}
	if err := cmd.Run(); err != nil {
		return nil, errors.New("PowerShell AST parser unavailable; use PowerShell with a direct $env:DEEPZORA_SECRET_NAME reference")
	}
	return parsePowerShellSecretNames(out.Bytes())
}
func parsePowerShellSecretNames(body []byte) ([]string, error) {
	var result struct {
		OK    bool     `json:"ok"`
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return nil, errors.New("cannot parse secret command; repair PowerShell syntax before using a secret")
	}
	names := []string{}
	for _, ref := range result.Names {
		name, ok := strings.CutPrefix(ref, "env:"+teamroster.SecretEnvPrefix)
		if !ok || name == "" || teamroster.NormalizeSecretName(name) != name {
			return nil, errors.New("invalid PowerShell parser result")
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

type boundedSecretParserOutput struct{ bytes.Buffer }

func (b *boundedSecretParserOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32<<10 {
		return 0, errors.New("PowerShell parser output exceeds budget")
	}
	return b.Buffer.Write(p)
}
