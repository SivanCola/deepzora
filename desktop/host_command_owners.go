package service

import (
	_ "embed"
	"encoding/json"

	"deepzora/desktop/internal/hostrpc"
)

// The shipped contract names this package's DTOs main.*, from when the
// application was the executable's package main. Renaming them is a protocol
// change, so the wire name stays fixed independently of the Go package.
const hostWirePackage = "main"

const hostCommandOwnersFile = "host_command_owners.generated.json"

//go:embed host_command_owners.generated.json
var hostCommandOwnersJSON []byte

func newDesktopRegistry(app *App) (*hostrpc.Registry, error) {
	var owners map[string]hostrpc.CommandOwnership
	if err := json.Unmarshal(hostCommandOwnersJSON, &owners); err != nil {
		return nil, err
	}
	return hostrpc.NewWireRegistry(app, nil, owners, hostWirePackage)
}
