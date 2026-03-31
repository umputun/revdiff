package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	Ref struct {
		Ref string `positional-arg-name:"ref" description:"git ref to diff against (default: uncommitted changes)"`
	} `positional-args:"yes"`

	Staged  bool `long:"staged" description:"show staged changes"`
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Version bool `short:"V" long:"version" description:"show version info"`
}

var revision = "unknown"

func main() {
	fmt.Printf("revdiff %s\n", revision)

	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[OPTIONS] [ref]"
	if _, err := p.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("version: %s\ngo: %s\n", revision, runtime.Version())
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// placeholder for bubbletea app initialization
	return nil
}
