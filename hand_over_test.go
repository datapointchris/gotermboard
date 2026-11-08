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
	return m.HandOver(exec.Command("sh", "-c", start.script), func(_ *Model, err error, stderr []byte) tea.Cmd {
		*b.said, *b.err = LastLine(stderr, err), err
		return tea.Quit
	})
}

func TestAHandoverKeepsStderrAndComesBackOnlyToTheTabThatHandedOver(t *testing.T) {
	for _, tc := range []struct {
		name, script, said  string
		failed, leavesChild bool
	}{
		{
			name: "a failure says its last stderr line", script: "printf 'warming up\\nthe stand-in failed\\n' >&2; exit 3",
			said: "the stand-in failed", failed: true,
		},
		{
			// An editor that starts a server and exits leaves it writing to stderr.
			name: "a child left writing stderr after the program exits holds nothing up and lives", leavesChild: true,
			script: "(sleep 1; echo late >&2; touch ALIVE) >/dev/null & exit 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alive := filepath.Join(t.TempDir(), "alive")
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
			go program.Send(startHandOver{strings.ReplaceAll(tc.script, "ALIVE", alive)})
			if _, runErr := program.Run(); runErr != nil {
				t.Fatalf("the board never came back from the stand-in: %v", runErr)
			}
			if said != tc.said || (err != nil) != tc.failed {
				t.Errorf("then was told %q and %v, want %q and failed=%v", said, err, tc.said, tc.failed)
			}
			if len(handedOver) != 1 || len(other) != 0 {
				t.Errorf("the handback reached the tab that handed over %d times and the other %d, want once and never", len(handedOver), len(other))
			}
			for tc.leavesChild && ctx.Err() == nil {
				if _, statErr := os.Stat(alive); statErr == nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			if tc.leavesChild {
				t.Error("the child the program left died writing to stderr after the handback")
			}
		})
	}
}

func TestOpenWindowLooksForItsProgramsAndDirectoryThenRunsArgvInTmux(t *testing.T) {
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

	brief := []string{"claude", "a brief with spaces"}
	if err := OpenWindow("claude 12", dir, brief); err == nil || !strings.Contains(err.Error(), "not on PATH: claude") {
		t.Errorf("a window whose program is not on PATH opened, or said %v", err)
	}
	standIn("claude", "")
	// A checkout deleted since the board read it, where tmux would open the window elsewhere.
	if err := OpenWindow("claude 12", filepath.Join(dir, "gone"), brief); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("a window whose directory is gone opened, or said %v", err)
	}
	if got := ran(); got != "" {
		t.Errorf("tmux ran for a window refused:\n%s", got)
	}

	if err := OpenWindow("claude 12", dir, brief); err != nil {
		t.Fatal(err)
	}
	if got, want := ran(), strings.Join([]string{"new-window", "-n", "claude 12", "-c", dir, "claude", "a brief with spaces"}, "\n")+"\n"; got != want {
		t.Errorf("tmux ran\n%s\nwant\n%s", got, want)
	}

	shellCommand := []string{"unlisted '/review-pr 12'"}
	if err := OpenWindow("review", "", shellCommand, "unlisted"); err == nil || !strings.Contains(err.Error(), "not on PATH: unlisted") {
		t.Errorf("a shell command whose needs are not on PATH opened, or said %v", err)
	}
	standIn("unlisted", "")
	if err := OpenWindow("review", "", shellCommand, "unlisted"); err != nil {
		t.Errorf("a lone shell command was looked for on PATH whole: %v", err)
	}
	if got, want := ran(), "new-window\n-n\nreview\nunlisted '/review-pr 12'\n"; got != want {
		t.Errorf("tmux ran\n%s\nwant\n%s", got, want)
	}
}
