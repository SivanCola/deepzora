package service

import (
	"io"
	"log"

	"deepzora/internal/secrets"
)

// The process entry owns the log writer; individual App fixtures must not
// replace a process-global writer. The stream refreshes masks on every write.
func runMaskedHostRPC(app *App, stdin io.Reader, stdout io.Writer) int {
	app.ensureTeamSecretMasking()
	previous := log.Writer()
	stream := secrets.NewStream(previous)
	log.SetOutput(stream)
	defer func() { _ = stream.Flush(); log.SetOutput(previous) }()
	return runHostRPC(app, stdin, stdout)
}
