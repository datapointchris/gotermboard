// read-markdown draws a markdown file, or what is piped to it, on a full-screen reader.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermboard"
	"github.com/datapointchris/gotermboard/markdown"
	"github.com/datapointchris/gotermstyle"
	"golang.org/x/term"
)

const usage = `read-markdown draws markdown on a full-screen reader

Usage:
  read-markdown [file]
  command | read-markdown

With no file, or with -, it reads standard input. Where standard output is no
terminal, it prints the whole document at --width and exits.

Flags:
  --width int   Columns to print at where standard output is no terminal (default 100)
  --version     Print the version and exit
  --help        Print this and exit

Keys:
  j k, arrows         scroll a line
  ctrl+d ctrl+u       half a screen
  g G                 top, bottom
  [ ]                 previous, next heading
  / n N               search, next match, previous match
  ?                   every key
  q                   quit
`

func main() {
	flags := flag.NewFlagSet("read-markdown", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	width := flags.Int("width", 100, "")
	version := flags.Bool("version", false, "")
	err := flags.Parse(os.Args[1:])
	switch {
	case errors.Is(err, flag.ErrHelp):
		fmt.Print(usage)
		return
	case err != nil:
		fmt.Fprintf(os.Stderr, "read-markdown: %v\n\n%s", err, usage)
		os.Exit(2)
	case *version:
		fmt.Println("read-markdown", builtVersion())
		return
	case flags.NArg() > 1:
		fmt.Fprintf(os.Stderr, "read-markdown: takes one file, given %d\n\n%s", flags.NArg(), usage)
		os.Exit(2)
	case *width < 1:
		fmt.Fprintf(os.Stderr, "read-markdown: --width must be at least 1, given %d\n", *width)
		os.Exit(2)
	}
	if err := run(flags.Arg(0), *width); err != nil {
		fmt.Fprintln(os.Stderr, "read-markdown:", err)
		os.Exit(1)
	}
}

func run(path string, width int) error {
	title, source, err := read(path)
	if err != nil {
		return err
	}
	renderer := markdown.New(nil)
	reader := gotermboard.NewReader(title, func(width int) []gotermboard.Line {
		return renderer.Lines(source, width)
	}, gotermstyle.PaletteFor(os.Stdout))
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		_, err := fmt.Print(reader.Once(width))
		return err
	}
	_, err = tea.NewProgram(reader).Run()
	return err
}

func read(path string) (title, source string, err error) {
	if path == "" || path == "-" {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return "", "", errors.New("no file named and nothing piped in")
		}
		content, err := io.ReadAll(os.Stdin)
		return "standard input", string(content), err
	}
	content, err := os.ReadFile(path)
	return filepath.Base(path), string(content), err
}

func builtVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}
