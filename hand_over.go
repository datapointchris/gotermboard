package gotermboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// EditorCommand is the user's editor on path: $VISUAL, then $EDITOR, then vi, split on
// spaces, so EDITOR="code --wait" works. A board's key hands it the terminal with
// Model.HandOver, as the Reader's e does with tea.ExecProcess.
func EditorCommand(path string) *exec.Cmd {
	command := []string{"vi"}
	for _, variable := range []string{"VISUAL", "EDITOR"} {
		if configured := strings.Fields(os.Getenv(variable)); len(configured) > 0 {
			command = configured
			break
		}
	}
	return exec.Command(command[0], append(command[1:], path)...)
}

// HandedBack is what a program Model.HandOver gave the terminal to sends once it exits. Only
// the model that handed it over takes it, so among Tabs the tab whose key started it runs its
// then, hidden or not, and no other tab does. That model's board receives it first through
// ReceivingBoard, to read again what the program may have changed.
type HandedBack struct {
	// Err is how the program ended, nil where it exited 0.
	Err error
	// Stderr is what the program wrote to stderr, which reached the terminal too as it ran.
	Stderr []byte

	model *Model
	then  func(m *Model, err error, stderr []byte) tea.Cmd
}

// HandOver gives the terminal to cmd until it exits, then runs then with how it ended and what
// it wrote to stderr. LastLine(stderr, err) is the sentence a failure says.
func (m *Model) HandOver(cmd *exec.Cmd, then func(m *Model, err error, stderr []byte) tea.Cmd) tea.Cmd {
	stderr := keepStderr(cmd)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return HandedBack{Err: exitOf(err), Stderr: stderr.Bytes(), model: m, then: then}
	})
}

// keepStderr copies what cmd writes to stderr into the buffer it returns, as well as to where
// it was going. A child the program leaves running with stderr open holds the handback a
// second at most.
func keepStderr(cmd *exec.Cmd) *bytes.Buffer {
	var kept bytes.Buffer
	terminal := cmd.Stderr
	if terminal == nil {
		terminal = os.Stderr
	}
	cmd.Stderr = io.MultiWriter(terminal, &kept)
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = time.Second
	}
	return &kept
}

// exitOf is how a program ended. A program that exited 0 and left a child holding its stderr
// ended well, so the wait cut short on that child is not its failure.
func exitOf(err error) error {
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}

// windowTimeout bounds tmux opening a window, which answers at once on a live server.
const windowTimeout = 5 * time.Second

// OpenWindow runs argv in a new tmux window named name, in dir where dir is not empty, and
// leaves the terminal with the board. tmux runs several arguments with no shell, so none needs
// quoting, and runs a lone one through its default-shell. tmux opens the window whether or not
// its program starts, so tmux and argv[0] are looked for on PATH first. A lone argument holding
// a space is a shell command rather than a program, so its caller looks for what it runs.
//
// Outside tmux it opens nothing, because tmux would open the window in whichever session it
// last used.
func OpenWindow(name, dir string, argv ...string) error {
	if len(argv) == 0 {
		return fmt.Errorf("nothing to run in a tmux window named %s", name)
	}
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("%s opens in a tmux window, and %s is not running inside tmux", name, filepath.Base(os.Args[0]))
	}
	programs := []string{"tmux"}
	if len(argv) > 1 || !strings.ContainsFunc(argv[0], unicode.IsSpace) {
		programs = append(programs, argv[0])
	}
	for _, program := range programs {
		if _, err := exec.LookPath(program); err != nil {
			return fmt.Errorf("%s is not on PATH, so no window opened for %s", program, name)
		}
	}
	args := []string{"new-window", "-n", name}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), windowTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "tmux", append(args, argv...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux could not open %s: %s", name, LastLine(out, err))
	}
	return nil
}

// LastLine is the last line a failed command wrote, or how it ended where it wrote nothing.
func LastLine(out []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	if err == nil {
		return ""
	}
	return err.Error()
}
