package gotermboard

import (
	"fmt"
	"os"
	"os/exec"
	"slices"

	tea "charm.land/bubbletea/v2"
)

// ClaudeSession is a Claude session a board's C opens on one record.
type ClaudeSession struct {
	// On names the record as its handle on the command line, such as an issue's number. The tmux
	// window is named "claude <On>", and every sentence about the session names it.
	On string
	// Dir is where the session runs: the record's checkout, or else the board's own directory.
	Dir string
	// Fallback is why Dir is not the record's checkout, as a bare clause completing "because",
	// such as "12 names no one repo". It is empty where Dir is the record's checkout.
	Fallback string
	// Brief says what the record is and the command that prints it whole, and asks nothing.
	// OpenClaudeSession adds the ask after it, and with a Fallback where the session runs and
	// why, because that directory may hold another repo's code.
	Brief string
	// Env is KEY=value pairs the session starts with on top of the program's environment. Inside
	// tmux they go ahead of claude through env, because a window takes its environment from the
	// tmux server.
	Env []string
}

// sessionAsk follows every brief: the session reads the record before it changes anything.
const sessionAsk = "Read it, say what it asks for and what you would do, then wait before changing anything."

// brief is the session's first message whole.
func (s ClaudeSession) brief() string {
	brief := s.Brief + " " + sessionAsk
	if s.Fallback != "" {
		brief += fmt.Sprintf(" This session runs in %s, because %s.", s.Dir, s.Fallback)
	}
	return brief
}

// ranIn is where the session runs, with why where it is not the record's checkout.
func (s ClaudeSession) ranIn() string {
	if s.Fallback == "" {
		return s.Dir
	}
	return s.Dir + ", because " + s.Fallback
}

// ClaudeSessionOutcome is what came of OpenClaudeSession. A Model says Said on its footer while
// it shows, then hands the outcome to its ReceivingBoard. The tab showing is the tab whose key
// opened the session, because a session holding the terminal leaves no key to change tabs with,
// and tmux makes the window it opens current. A host draws Said where it says what a key did.
type ClaudeSessionOutcome struct {
	Session ClaudeSession
	// Said is the sentence saying what came of it.
	Said string
	// Err is why no session opened, or how one that held the terminal failed. It is nil where a
	// window opened, or the session ended 0.
	Err error
	// HeldTerminal says the session held the terminal and has ended, so what it changed is there
	// to read again.
	HeldTerminal bool
	// Stderr is the end of what a session that held the terminal wrote to stderr.
	Stderr []byte
}

// OpenClaudeSession is C: a Claude session on a record, in session.Dir. Inside tmux it opens in a
// window of its own, so the board stays usable while the session works. tmux answers once the
// window exists, before claude starts, so the outcome says only that the window opened. Outside
// tmux it holds the terminal until it ends, and a failure says the last line it wrote to stderr.
// The command ends in a ClaudeSessionOutcome either way, so a Model and a host that draws its own
// screen open it alike.
func OpenClaudeSession(session ClaudeSession) tea.Cmd {
	outcome := ClaudeSessionOutcome{Session: session}
	if _, err := exec.LookPath("claude"); err != nil {
		outcome.Err, outcome.Said = err, "claude is not on PATH, so no session opened on "+session.On
		return func() tea.Msg { return outcome }
	}
	argv := []string{"claude", session.brief()}
	if os.Getenv("TMUX") != "" {
		if len(session.Env) > 0 {
			argv = slices.Concat([]string{"env"}, session.Env, argv)
		}
		return func() tea.Msg {
			if err := OpenWindow("claude "+session.On, session.Dir, argv); err != nil {
				outcome.Err, outcome.Said = err, err.Error()
				return outcome
			}
			outcome.Said = fmt.Sprintf("opened a tmux window for the Claude session on %s, in %s", session.On, session.ranIn())
			return outcome
		}
	}
	held := exec.Command(argv[0], argv[1:]...)
	held.Dir = session.Dir
	if len(session.Env) > 0 {
		held.Env = append(os.Environ(), session.Env...)
	}
	return handOver(&handedOver{cmd: held}, func(err error, stderr []byte) tea.Msg {
		outcome.HeldTerminal, outcome.Err, outcome.Stderr = true, err, stderr
		switch {
		case err != nil:
			outcome.Said = fmt.Sprintf("the Claude session on %s ended: %s", session.On, LastLine(stderr, err))
		case session.Fallback != "":
			outcome.Said = fmt.Sprintf("back from the Claude session on %s, which ran in %s", session.On, session.ranIn())
		default:
			outcome.Said = "back from the Claude session on " + session.On
		}
		return outcome
	})
}
