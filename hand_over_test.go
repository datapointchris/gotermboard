package gotermboard

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

type startHandOver struct{ script string }

type handedBackTo struct {
	fixedBoard
	received *[]HandedBack
}

func (b handedBackTo) Receive(_ *Model, msg tea.Msg) tea.Cmd {
	if back, ok := msg.(HandedBack); ok {
		*b.received = append(*b.received, back)
	}
	return nil
}

// handingOver hands the terminal to a stand-in script on startHandOver, and quits once it is
// back, keeping what then was told.
type handingOver struct {
	handedBackTo
	said *string
	err  *error
}

func (b handingOver) Receive(m *Model, msg tea.Msg) tea.Cmd {
	start, ok := msg.(startHandOver)
	if !ok {
		return b.handedBackTo.Receive(m, msg)
	}
	standIn := exec.Command("sh", "-c", start.script)
	standIn.Stderr = io.Discard
	return m.HandOver(standIn, func(_ *Model, err error, stderr []byte) tea.Cmd {
		*b.said, *b.err = LastLine(stderr, err), err
		return tea.Quit
	})
}

func TestAHandoverKeepsStderrAndComesBackOnlyToTheTabThatHandedOver(t *testing.T) {
	for _, tc := range []struct {
		name, script, said string
		failed             bool
	}{
		{
			name: "a failure says its last stderr line", script: "printf 'warming up\\nthe stand-in failed\\n' >&2; exit 3",
			said: "the stand-in failed", failed: true,
		},
		{name: "a child left holding stderr holds nothing up", script: "sleep 10 & exit 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var said string
			var err error
			var handedOver, other []HandedBack
			board := handingOver{handedBackTo{fixedBoard{}, &handedOver}, &said, &err}
			tabs, newErr := NewTabs([]Tab{{Name: "hands over", Board: board}, {Name: "other", Board: handedBackTo{fixedBoard{}, &other}}},
				gotermstyle.NewPalette(false), "")
			if newErr != nil {
				t.Fatal(newErr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			program := tea.NewProgram(tabs, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithContext(ctx))
			go program.Send(startHandOver{tc.script})
			if _, runErr := program.Run(); runErr != nil {
				t.Fatalf("the board never came back from the stand-in: %v", runErr)
			}
			if said != tc.said || (err != nil) != tc.failed {
				t.Errorf("then was told %q and %v, want %q and failed=%v", said, err, tc.said, tc.failed)
			}
			if len(handedOver) != 1 || len(other) != 0 {
				t.Errorf("the handback reached the tab that handed over %d times and the other %d, want once and never", len(handedOver), len(other))
			}
		})
	}
}

func TestOpenWindowLooksForItsProgramThenRunsArgvInTmux(t *testing.T) {
	bin, dir := t.TempDir(), t.TempDir()
	recorded := filepath.Join(t.TempDir(), "tmux-args")
	standIn := func(name, body string) {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	standIn("tmux", `for arg in "$@"; do printf '%s\n' "$arg"; done >> `+recorded+"\n")
	t.Setenv("PATH", bin)
	t.Setenv("TMUX", "/tmp/stand-in,1,0")
	ran := func() string {
		out, _ := os.ReadFile(recorded)
		_ = os.Remove(recorded)
		return string(out)
	}

	if err := OpenWindow("claude 12", dir, "claude", "a brief with spaces"); err == nil || !strings.Contains(err.Error(), "claude is not on PATH") {
		t.Errorf("a window whose program is not on PATH opened, or said %v", err)
	}
	if got := ran(); got != "" {
		t.Errorf("tmux ran for a program not on PATH:\n%s", got)
	}

	standIn("claude", "")
	if err := OpenWindow("claude 12", dir, "claude", "a brief with spaces"); err != nil {
		t.Fatal(err)
	}
	if got, want := ran(), strings.Join([]string{"new-window", "-n", "claude 12", "-c", dir, "claude", "a brief with spaces"}, "\n")+"\n"; got != want {
		t.Errorf("tmux ran\n%s\nwant\n%s", got, want)
	}

	if err := OpenWindow("review", "", "unlisted '/review-pr 12'"); err != nil {
		t.Errorf("a lone shell command was looked for on PATH: %v", err)
	}
	if got, want := ran(), "new-window\n-n\nreview\nunlisted '/review-pr 12'\n"; got != want {
		t.Errorf("tmux ran\n%s\nwant\n%s", got, want)
	}
}
