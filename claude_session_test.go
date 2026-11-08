package gotermboard

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

// standInPath puts stand-in programs on a PATH holding nothing else.
func standInPath(t *testing.T, programs map[string]string) {
	t.Helper()
	bin := t.TempDir()
	for name, body := range programs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func TestAClaudeSessionInTmuxOpensAWindowNamedForItsRecordWithTheBriefWhole(t *testing.T) {
	dir, recorded := t.TempDir(), filepath.Join(t.TempDir(), "tmux-args")
	session := ClaudeSession{On: "12", Dir: dir, Fallback: "12 names no one repo", Brief: "This is issue 12.", Env: []string{"WORK=issue 12"}}
	t.Setenv("TMUX", "/tmp/stand-in,1,0")

	standInPath(t, map[string]string{"tmux": ""})
	if outcome := OpenClaudeSession(session)().(ClaudeSessionOutcome); outcome.Err == nil {
		t.Errorf("a session with no claude on PATH opened, saying %q", outcome.Said)
	}

	standInPath(t, map[string]string{"claude": "", "env": "", "tmux": `for arg in "$@"; do printf '%s\n' "$arg"; done > ` + recorded + "\n"})
	outcome := OpenClaudeSession(session)().(ClaudeSessionOutcome)
	if outcome.Err != nil || outcome.HeldTerminal {
		t.Fatalf("the window did not open: %v", outcome.Err)
	}
	if !strings.Contains(outcome.Said, dir) || !strings.Contains(outcome.Said, "12 names no one repo") {
		t.Errorf("said %q, which names neither where the session runs nor why", outcome.Said)
	}
	ran, _ := os.ReadFile(recorded)
	brief := "This is issue 12. " + sessionAsk + " This session runs in " + dir + ", because 12 names no one repo."
	if got, want := string(ran), strings.Join([]string{"new-window", "-n", "claude 12", "-c", dir, "env", "WORK=issue 12", "claude", brief}, "\n")+"\n"; got != want {
		t.Errorf("tmux ran\n%s\nwant\n%s", got, want)
	}
}

type startClaudeSession struct{}

// openingClaude opens a session on startClaudeSession where it has one, and quits once an
// outcome reaches it.
type openingClaude struct {
	fixedBoard
	session  *ClaudeSession
	received *[]ClaudeSessionOutcome
}

func (b openingClaude) Receive(_ *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case startClaudeSession:
		if b.session != nil {
			return OpenClaudeSession(*b.session)
		}
	case ClaudeSessionOutcome:
		*b.received = append(*b.received, msg)
		return tea.Quit
	}
	return nil
}

func TestAClaudeSessionOutsideTmuxHoldsTheTerminalAndItsTabSaysItsLastStderrLine(t *testing.T) {
	dir, ran := t.TempDir(), t.TempDir()
	t.Setenv("TMUX", "")
	standInPath(t, map[string]string{"claude": `pwd > ` + ran + `/dir; printf '%s' "$1" > ` + ran + `/brief
printf 'warming up\nno credits left\n' >&2; exit 3`})

	var showing, hidden []ClaudeSessionOutcome
	session := ClaudeSession{On: "12", Dir: dir, Brief: "This is issue 12."}
	tabs, err := NewTabs([]Tab{
		{Name: "opens", Board: openingClaude{session: &session, received: &showing}},
		{Name: "other", Board: openingClaude{received: &hidden}},
	}, gotermstyle.NewPalette(false), "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	program := tea.NewProgram(tabs, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithContext(ctx))
	go program.Send(startClaudeSession{})
	if _, err := program.Run(); err != nil {
		t.Fatalf("the board never came back from the session: %v", err)
	}

	if len(showing) != 1 || len(hidden) != 0 {
		t.Fatalf("the outcome reached the tab showing %d times and the hidden one %d, want once and never", len(showing), len(hidden))
	}
	outcome := showing[0]
	if !outcome.HeldTerminal || !strings.Contains(outcome.Said, "no credits left") || tabs.active().said != outcome.Said {
		t.Errorf("the outcome said %q and the footer %q, want the last stderr line from a session that held the terminal", outcome.Said, tabs.active().said)
	}
	if got, _ := os.ReadFile(filepath.Join(ran, "dir")); strings.TrimSpace(string(got)) != dir {
		t.Errorf("the session ran in %s, want %s", got, dir)
	}
	if got, _ := os.ReadFile(filepath.Join(ran, "brief")); string(got) != "This is issue 12. "+sessionAsk {
		t.Errorf("the session was briefed %q", got)
	}
}
