package weboptions

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const Usage = `Usage: deepzora web [--port PORT] [--no-open] [--dir PATH]

Start the local Web workspace using the desktop UI and real Go backend.
  --port PORT   Listen on 127.0.0.1:PORT (default 0: choose a free port)
  --no-open     Print the access URL without opening a browser
  --dir PATH    Initial project directory (default: current directory)
Stop with Ctrl-C. Closing the browser does not stop running tasks.
`

type Options struct {
	Port   int
	NoOpen bool
	Dir    string
	Help   bool
}

func ParseOptions(args []string) (Options, error) {
	var o Options
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&o.Port, "port", 0, "loopback port")
	fs.BoolVar(&o.NoOpen, "no-open", false, "do not open browser")
	fs.StringVar(&o.Dir, "dir", ".", "initial project directory")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			o.Help = true
			return o, nil
		}
		return o, err
	}
	if fs.NArg() != 0 {
		return o, errors.New("unexpected positional arguments; use --dir PATH")
	}
	if o.Port < 0 || o.Port > 65535 {
		return o, errors.New("port must be between 0 and 65535")
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return o, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return o, err
	}
	if !info.IsDir() {
		return o, fmt.Errorf("not a directory: %s", dir)
	}
	o.Dir = dir
	return o, nil
}
