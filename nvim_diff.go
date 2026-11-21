package gotermboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The variables nvim's diff starts with, each naming a file NvimDiff.Write wrote. An nvim config
// that finds them shows the document beside the diff and each thread on its line. An nvim config
// reads these names, so renaming one breaks every config that reads it.
const (
	NvimDiffDocumentVariable = "PRS_DIFF_DOCUMENT"
	NvimDiffThreadsVariable  = "PRS_DIFF_THREADS"
)

// The statuses nvim's diff exits with where it shows no diff, so a board can say why rather than
// leave an empty buffer.
const (
	// NvimDiffNoDiffview is the status where :DiffviewOpen is not defined. A plugin that loads on
	// its command still defines it.
	NvimDiffNoDiffview = 3
	// NvimDiffNoTab is the status where Diffview opened no tab.
	NvimDiffNoTab = 4
)

// NvimDiffNoDiffviewSaid is why nvim showed no diff where it exited NvimDiffNoDiffview.
const NvimDiffNoDiffviewSaid = "nvim has no :DiffviewOpen, and the diff needs the diffview.nvim plugin"

// NvimDiffThreadsSchema is the JSON Schema of the file NvimDiffThreadsVariable names.
//
//go:embed nvim-diff-threads.schema.json
var NvimDiffThreadsSchema []byte

// NvimDiffKind is which diff opened, as the threads file names it.
type NvimDiffKind string

const (
	// NvimDiffWhole runs three-dot from where a pull request's branch left its base, so its left
	// side is the base branch's copy, where a LEFT thread's line counts.
	NvimDiffWhole NvimDiffKind = "whole"
	// NvimDiffSinceReview runs from the commit the newest review read, which is its left side.
	NvimDiffSinceReview NvimDiffKind = "since_review"
	// NvimDiffSession is what sessions made, a tab per repo, with no pull request.
	NvimDiffSession NvimDiffKind = "session"
)

// NvimDiffThreads is the threads file, in the shape NvimDiffThreadsSchema gives it. Slug, Number,
// URL, Base and Head describe a pull request, and Sessions what a session diff shows.
type NvimDiffThreads struct {
	Diff     NvimDiffKind `json:"diff"`
	Title    string       `json:"title"`
	Slug     string       `json:"slug,omitempty"`
	Number   int          `json:"number,omitempty"`
	URL      string       `json:"url,omitempty"`
	Base     string       `json:"base,omitempty"`
	Head     string       `json:"head,omitempty"`
	Sessions []string     `json:"sessions,omitempty"`
	// ThreadsRead is false where the writer held no read of the threads, so an empty Threads
	// says nothing. ThreadsLeftOut counts those on a side the diff does not show.
	ThreadsRead    bool             `json:"threads_read"`
	Threads        []NvimDiffThread `json:"threads"`
	ThreadsLeftOut int              `json:"threads_left_out"`
}

// NvimDiffThread is one review thread on a line of a file. Line is null where the thread's line
// is gone from the diff. Side is LEFT where Line counts in the base's copy and RIGHT in the head's.
type NvimDiffThread struct {
	Path         string            `json:"path"`
	Line         *int              `json:"line"`
	Side         string            `json:"side"`
	Resolved     bool              `json:"resolved"`
	Outdated     bool              `json:"outdated"`
	Comments     []NvimDiffComment `json:"comments"`
	CommentCount int               `json:"comment_count"`
}

// NvimDiffComment is one comment in a thread, oldest first.
type NvimDiffComment struct {
	Author            string    `json:"author"`
	AuthorType        string    `json:"author_type"`
	AuthorAssociation string    `json:"author_association"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
	URL               string    `json:"url"`
}

// NvimDiffTab is one Diffview tab: Revisions as DiffviewOpen takes them, compared in Checkout.
// Label names the tab where Diffview opens none.
type NvimDiffTab struct {
	Label     string
	Checkout  string
	Revisions string
}

// NvimDiff is a diff to hand the terminal to nvim for: a Diffview tab for each of Tabs, with
// Document and Threads beside it. Subject names it on its tmux window and its socket, such as
// `widget #13` or `session 8c5cbcad`. With no Document, Write writes neither file and nvim starts
// with neither variable, as for Diffview's resolver on a merge's conflicts.
type NvimDiff struct {
	Subject  string
	Tabs     []NvimDiffTab
	Document string
	Threads  NvimDiffThreads
}

// NvimDiffHandover is an NvimDiff whose files are written, ready to run. The script, the
// document and the threads file sit in a directory of their own, which Ended removes. The socket
// sits in the temporary directory as prs-diff-<stem>.sock, where stem is the subject with each
// run of characters other than letters, digits, `.`, `_` and `-` made one `-`. A program in a
// window named for the subject, such as `diff <subject>` or `explain <subject>`, finds the
// socket from that name.
type NvimDiffHandover struct {
	subject, socket, files, checkout string
	documented                       bool
}

// Write writes the script nvim runs, and the document and threads file where there is a
// Document. It refuses where an nvim still listens on the subject's socket, and removes a socket
// an nvim that ended left there. A socket path longer than a unix socket holds gets no socket,
// and the diff opens without one.
func (d NvimDiff) Write() (*NvimDiffHandover, error) {
	if len(d.Tabs) == 0 {
		return nil, fmt.Errorf("the diff of %s names no tab to open", d.Subject)
	}
	socket := nvimDiffSocket(d.Subject)
	if socket != "" {
		if listening(socket) {
			return nil, fmt.Errorf("the diff of %s is already open in the nvim listening on %s", d.Subject, socket)
		}
		_ = os.Remove(socket)
	}
	files, err := os.MkdirTemp("", "nvim-diff-*")
	if err != nil {
		return nil, err
	}
	h := &NvimDiffHandover{subject: d.Subject, socket: socket, files: files, checkout: d.Tabs[0].Checkout, documented: d.Document != ""}
	err = os.WriteFile(h.file("diff.vim"), []byte(d.script(h.file("unopened"))), 0o600)
	if err == nil && h.documented {
		var encoded []byte
		if encoded, err = json.MarshalIndent(d.Threads.withLists(), "", "  "); err == nil {
			err = errors.Join(
				os.WriteFile(h.file("document.md"), []byte(d.Document), 0o600),
				os.WriteFile(h.file("threads.json"), append(encoded, '\n'), 0o600),
			)
		}
	}
	if err != nil {
		_ = os.RemoveAll(files)
		return nil, err
	}
	return h, nil
}

// withLists is t with an empty list for every list it left nil, since the schema reads a list
// and JSON writes a nil one as null.
func (t NvimDiffThreads) withLists() NvimDiffThreads {
	threads := make([]NvimDiffThread, 0, len(t.Threads))
	for _, thread := range t.Threads {
		if thread.Comments == nil {
			thread.Comments = []NvimDiffComment{}
		}
		threads = append(threads, thread)
	}
	t.Threads = threads
	return t
}

// script opens a Diffview tab for each of d's tabs, then closes the empty tab nvim started on.
// Diffview logs a refusal rather than throwing, so an open that left the tab count as it was is
// unopened too. An open made while the tab before it still reads its files can find no git
// directory, so each is tried twice. Each unopened tab goes to the unopened file as its label, a
// tab and why, and nvim exits NvimDiffNoTab where none opened.
func (d NvimDiff) script(unopened string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `if exists(':DiffviewOpen') != 2
  cquit %d
endif
let s:unopened = []
function! s:open(label, checkout, revisions) abort
  let tabs = tabpagenr('$')
  try
    for attempt in [1, 2]
      execute 'DiffviewOpen -C' .. fnameescape(a:checkout) .. ' ' .. a:revisions
      if tabpagenr('$') > tabs
        return
      endif
    endfor
  catch
    call add(s:unopened, a:label .. "\t" .. v:exception)
    return
  endtry
  call add(s:unopened, a:label .. "\tDiffview opened no tab, and :DiffviewLog says why")
endfunction
`, NvimDiffNoDiffview)
	for _, tab := range d.Tabs {
		fmt.Fprintf(&b, "call s:open(%s, %s, %s)\n", vimQuoted(tab.Label), vimQuoted(tab.Checkout), vimQuoted(tab.Revisions))
	}
	fmt.Fprintf(&b, `call writefile(s:unopened, %s)
if tabpagenr('$') == 1
  cquit %d
endif
1tabclose
tabfirst
`, vimQuoted(unopened), NvimDiffNoTab)
	return b.String()
}

// Argv is nvim listening on the socket and running the script.
func (h *NvimDiffHandover) Argv() []string {
	if h.socket == "" {
		return []string{"nvim", "-S", h.file("diff.vim")}
	}
	return []string{"nvim", "--listen", h.socket, "-S", h.file("diff.vim")}
}

// Dir is the first tab's checkout, where nvim runs.
func (h *NvimDiffHandover) Dir() string { return h.checkout }

// Env is the variables naming the document and the threads file, as exec.Cmd's Env takes
// them, and nil with no Document.
func (h *NvimDiffHandover) Env() []string {
	if !h.documented {
		return nil
	}
	return []string{NvimDiffDocumentVariable + "=" + h.file("document.md"), NvimDiffThreadsVariable + "=" + h.file("threads.json")}
}

// Command runs Argv in Dir with Env added, for Model.HandOver. nvim waits on a person, so ctx is
// usually context.Background().
func (h *NvimDiffHandover) Command(ctx context.Context) *exec.Cmd {
	argv := h.Argv()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = h.checkout
	cmd.Env = append(os.Environ(), h.Env()...)
	return cmd
}

// NameWindow names the board's tmux window `diff <subject>` until restore runs. Outside tmux it
// does nothing.
func (h *NvimDiffHandover) NameWindow() (restore func()) { return nameWindow("diff " + h.subject) }

// NvimDiffEnding is what nvim's script left once nvim exited.
type NvimDiffEnding struct {
	// Unopened is each tab Diffview opened none for.
	Unopened []NvimDiffUnopened
	// NoDiff is why nvim showed no diff, and empty where its status says it showed one.
	NoDiff string
}

// NvimDiffUnopened is a tab Diffview opened none for, by its label, and why.
type NvimDiffUnopened struct{ Label, Why string }

// Ended reads what the script left, given the status nvim exited with, then removes every file
// Write wrote.
func (h *NvimDiffHandover) Ended(status int) NvimDiffEnding {
	var ending NvimDiffEnding
	if raw, err := os.ReadFile(h.file("unopened")); err == nil {
		for line := range strings.Lines(string(raw)) {
			label, why, _ := strings.Cut(strings.TrimRight(line, "\n"), "\t")
			if label != "" {
				ending.Unopened = append(ending.Unopened, NvimDiffUnopened{Label: label, Why: why})
			}
		}
	}
	_ = os.RemoveAll(h.files)
	switch status {
	case NvimDiffNoDiffview:
		ending.NoDiff = NvimDiffNoDiffviewSaid
	case NvimDiffNoTab:
		var each []string
		for _, u := range ending.Unopened {
			each = append(each, u.Label+": "+u.Why)
		}
		ending.NoDiff = "Diffview opened no tab of the diff of " + h.subject + ": " + strings.Join(each, "; ")
	}
	return ending
}

func (h *NvimDiffHandover) file(name string) string { return filepath.Join(h.files, name) }

// socketPathMost is the longest path a unix socket takes: macOS holds 104 bytes with the closing
// NUL, and Linux 108.
const socketPathMost = 103

var outsideStem = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// nvimDiffSocket is the subject's socket, and empty where its path is longer than a unix socket
// takes.
func nvimDiffSocket(subject string) string {
	socket := filepath.Join(os.TempDir(), "prs-diff-"+strings.Trim(outsideStem.ReplaceAllString(subject, "-"), "-")+".sock")
	if len(socket) > socketPathMost {
		return ""
	}
	return socket
}

// listening is whether something accepts a connection on socket.
func listening(socket string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// vimQuoted is text as a single-quoted Vim string.
func vimQuoted(text string) string { return "'" + strings.ReplaceAll(text, "'", "''") + "'" }

// nameWindow names the tmux window the board runs in, and returns what puts its own name back. A
// rename turns automatic-rename off, so a window tmux named gets tmux's naming back.
func nameWindow(name string) (restore func()) {
	pane := os.Getenv("TMUX_PANE")
	if os.Getenv("TMUX") == "" || pane == "" {
		return func() {}
	}
	tmux := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), windowTimeout)
		defer cancel()
		return exec.CommandContext(ctx, "tmux", args...).Output()
	}
	out, err := tmux("display-message", "-p", "-t", pane, "#{window_name}\t#{automatic-rename}")
	own, automatic, found := strings.Cut(strings.TrimRight(string(out), "\n"), "\t")
	if err != nil || !found {
		return func() {}
	}
	if _, err := tmux("rename-window", "-t", pane, name); err != nil {
		return func() {}
	}
	return func() {
		if automatic == "1" {
			_, _ = tmux("set-option", "-w", "-t", pane, "automatic-rename", "on")
			return
		}
		_, _ = tmux("rename-window", "-t", pane, own)
	}
}
