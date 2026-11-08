package gotermboard

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	// Stderr is the end of what the program wrote to stderr, empty where the caller set
	// cmd.Stderr.
	Stderr []byte

	model *Model
	then  func(m *Model, err error, stderr []byte) tea.Cmd
}

// HandOver gives the terminal to cmd until it exits, then runs then with how it ended and the
// end of what it wrote to stderr. LastLine(stderr, err) is the sentence a failure says.
//
// To keep stderr, the program's fd 2 is a pipe whose bytes reach the terminal as they come, so
// a program that asks whether stderr is a terminal hears no. A caller handing over such a
// program sets cmd.Stderr, such as to os.Stderr, and keeps nothing.
func (m *Model) HandOver(cmd *exec.Cmd, then func(m *Model, err error, stderr []byte) tea.Cmd) tea.Cmd {
	return m.handOverProgram(&handedOver{cmd: cmd}, then)
}

// handOverProgram is HandOver for any program that keeps its stderr.
func (m *Model) handOverProgram(program stderrKeeper, then func(m *Model, err error, stderr []byte) tea.Cmd) tea.Cmd {
	return handOver(program, func(err error, stderr []byte) tea.Msg {
		return HandedBack{Err: err, Stderr: stderr, model: m, then: then}
	})
}

// stderrKeeper is a program Bubble Tea's Exec runs that keeps the end of its stderr.
type stderrKeeper interface {
	tea.ExecCommand
	kept() []byte
}

// handOver runs program on the terminal through Bubble Tea and ends in the message done makes.
func handOver(program stderrKeeper, done func(err error, stderr []byte) tea.Msg) tea.Cmd {
	return tea.Exec(program, func(err error) tea.Msg { return done(err, program.kept()) })
}

// keptStderr is how much of a handed-over program's stderr comes back, from its end.
const keptStderr = 64 << 10

// stderrDrainWait bounds the wait for the rest of stderr once the program exits. A child the
// program left running can hold stderr open long after.
const stderrDrainWait = 250 * time.Millisecond

// handedOver is a program as Bubble Tea's Exec runs it for handOver. Its stderr goes through a
// pipe of its own, drained to the terminal for as long as anything holds it open, so a child
// the program leaves running keeps writing to the terminal.
type handedOver struct {
	cmd      *exec.Cmd
	terminal io.Writer
	tail     stderrTail
}

// SetStdin is tea.ExecCommand's, and keeps a stdin the caller set.
func (h *handedOver) SetStdin(r io.Reader) {
	if h.cmd.Stdin == nil {
		h.cmd.Stdin = r
	}
}

// SetStdout is tea.ExecCommand's, and keeps a stdout the caller set.
func (h *handedOver) SetStdout(w io.Writer) {
	if h.cmd.Stdout == nil {
		h.cmd.Stdout = w
	}
}

// SetStderr is tea.ExecCommand's: w is the terminal the pipe drains to.
func (h *handedOver) SetStderr(w io.Writer) { h.terminal = w }

// Run is tea.ExecCommand's.
func (h *handedOver) Run() error {
	if h.terminal == nil {
		h.terminal = os.Stderr
	}
	if h.cmd.Stderr != nil {
		return h.cmd.Run()
	}
	drain, stderr, err := os.Pipe()
	if err != nil {
		h.cmd.Stderr = h.terminal
		return h.cmd.Run()
	}
	h.cmd.Stderr = stderr
	drained := make(chan struct{})
	go h.drain(drain, drained)
	err = h.cmd.Run()
	_ = stderr.Close()
	select {
	case <-drained:
	case <-time.After(stderrDrainWait):
	}
	return err
}

func (h *handedOver) drain(pipe *os.File, drained chan<- struct{}) {
	defer close(drained)
	defer func() { _ = pipe.Close() }()
	buf := make([]byte, 4096)
	for {
		n, err := pipe.Read(buf)
		if n > 0 {
			_, _ = h.terminal.Write(buf[:n])
			_, _ = h.tail.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (h *handedOver) kept() []byte { return h.tail.kept() }

// stderrTail keeps the last keptStderr bytes written to it.
type stderrTail struct {
	mu   sync.Mutex
	held []byte
}

func (s *stderrTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held = append(s.held, p...)
	if over := len(s.held) - keptStderr; over > 0 {
		s.held = s.held[over:]
	}
	return len(p), nil
}

func (s *stderrTail) kept() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.held)
}

// windowTimeout bounds tmux opening a window, which answers at once on a live server.
const windowTimeout = 5 * time.Second

// OpenWindow runs argv in a new tmux window named name, in dir where dir is not empty. tmux
// makes the window current, and the board keeps running in its own. tmux runs several
// arguments with no shell, so none needs quoting, and runs a lone one through its
// default-shell.
//
// tmux opens the window whether or not its program starts, and in another directory where dir
// is gone, so tmux, argv[0] and every program in needs are looked for on PATH first, and dir
// is looked for too. A lone argument holding a space is a shell command rather than a program,
// so needs names what it runs. Outside tmux it opens nothing, because tmux would open the
// window in whichever session it last used.
func OpenWindow(name, dir string, argv []string, needs ...string) error {
	if len(argv) == 0 {
		return fmt.Errorf("nothing to run in a tmux window named %s", name)
	}
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("%s opens in a tmux window, and %s is not running inside tmux", name, filepath.Base(os.Args[0]))
	}
	programs := append([]string{"tmux"}, needs...)
	if len(argv) > 1 || !strings.ContainsFunc(argv[0], unicode.IsSpace) {
		programs = append(programs, argv[0])
	}
	var missing []string
	for _, program := range programs {
		if _, err := exec.LookPath(program); err != nil && !slices.Contains(missing, program) {
			missing = append(missing, program)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("not on PATH: %s, so no window opened for %s", strings.Join(missing, ", "), name)
	}
	args := []string{"new-window", "-n", name}
	if dir != "" {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a directory, so no window opened for %s", dir, name)
		}
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
