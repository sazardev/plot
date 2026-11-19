package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

type appIO struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	InTTY   bool
	OutTTY  bool
	Width   int
	Height  int
	Env     map[string]string
	Profile termenv.Profile
}

func main() {
	os.Exit(run(os.Args[1:], systemIO()))
}

func systemIO() appIO {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width, height = 0, 0
	}
	return appIO{
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		InTTY:   term.IsTerminal(int(os.Stdin.Fd())),
		OutTTY:  term.IsTerminal(int(os.Stdout.Fd())),
		Width:   width,
		Height:  height,
		Env:     environ(),
		Profile: termenv.NewOutput(os.Stdout).Profile,
	}
}

func environ() map[string]string {
	env := make(map[string]string, len(os.Environ()))
	for _, entry := range os.Environ() {
		if i := strings.IndexByte(entry, '='); i >= 0 {
			env[entry[:i]] = entry[i+1:]
		}
	}
	return env
}

func versionText() string {
	if version == "" || version == "dev" {
		return "dev"
	}
	switch {
	case commit != "" && date != "":
		return fmt.Sprintf("%s (%s, %s)", version, commit, date)
	case commit != "":
		return fmt.Sprintf("%s (%s)", version, commit)
	default:
		return version
	}
}
