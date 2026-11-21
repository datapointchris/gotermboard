package gotermboard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A program split into the diff's window finds nvim's socket from the subject on the window's
// name, and nvim's script leaves each tab it opened none for, which Ended reads before it
// removes every file Write wrote.
func TestNvimDiffNamesItsSocketFromTheSubjectAndEndedReadsWhatTheScriptLeft(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	h, err := NvimDiff{
		Subject: "widget #13", Document: "# Count cache hits\n",
		Tabs: []NvimDiffTab{{Label: "widget", Checkout: "/src/widget", Revisions: "origin/main...refs/remotes/origin/pr/13"}},
	}.Write()
	if err != nil {
		t.Fatal(err)
	}
	stem := filepath.Join(os.TempDir(), "prs-diff-widget-13")
	if want := []string{"nvim", "--listen", stem + ".sock", "-S", stem + ".vim"}; !slices.Equal(h.Argv(), want) || h.Dir() != "/src/widget" {
		t.Errorf("nvim runs %q in %s, want %q in /src/widget", h.Argv(), h.Dir(), want)
	}
	if want := []string{"PRS_DIFF_DOCUMENT=" + stem + ".md", "PRS_DIFF_THREADS=" + stem + ".threads.json"}; !slices.Equal(h.Env(), want) {
		t.Errorf("nvim starts with %q, want %q", h.Env(), want)
	}
	script, _ := os.ReadFile(stem + ".vim")
	if !strings.Contains(string(script), "call s:open('widget', '/src/widget', 'origin/main...refs/remotes/origin/pr/13')") {
		t.Errorf("the script opens no tab of the diff:\n%s", script)
	}
	if threads, _ := os.ReadFile(stem + ".threads.json"); !strings.Contains(string(threads), `"threads": []`) {
		t.Errorf("a diff with no threads should write an empty list:\n%s", threads)
	}

	if err := os.WriteFile(stem+".unopened", []byte("widget\tVim:E492: Not an editor command\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ending := h.Ended(NvimDiffNoTab)
	if want := []NvimDiffUnopened{{Label: "widget", Why: "Vim:E492: Not an editor command"}}; !slices.Equal(ending.Unopened, want) ||
		ending.NoDiff != "Diffview opened no tab of the diff of widget #13: widget: Vim:E492: Not an editor command" {
		t.Errorf("Ended read %+v", ending)
	}
	if left, _ := filepath.Glob(stem + ".*"); len(left) > 0 {
		t.Errorf("Ended left %q", left)
	}
}
