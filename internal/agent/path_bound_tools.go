package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

var pathBoundWriterNames = map[string]bool{
	"write_file":    true,
	"edit_file":     true,
	"multi_edit":    true,
	"move_file":     true,
	"notebook_edit": true,
	"delete_range":  true,
	"delete_symbol": true,
}

func extractWritePathsFromArgs(toolName, workDir string, args json.RawMessage) ([]string, error) {
	switch toolName {
	case "move_file":
		var p struct {
			SourcePath      string `json:"source_path"`
			DestinationPath string `json:"destination_path"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if strings.TrimSpace(p.SourcePath) == "" || strings.TrimSpace(p.DestinationPath) == "" {
			return nil, fmt.Errorf("source_path and destination_path are required")
		}
		return []string{
			resolveMaybeRelative(workDir, p.SourcePath),
			resolveMaybeRelative(workDir, p.DestinationPath),
		}, nil
	default:
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		if strings.TrimSpace(p.Path) == "" {
			return nil, fmt.Errorf("path is required")
		}
		return []string{resolveMaybeRelative(workDir, p.Path)}, nil
	}
}

func resolveMaybeRelative(workDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	if strings.TrimSpace(workDir) == "" {
		return path
	}
	return filepath.Join(workDir, path)
}
