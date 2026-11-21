package gotermboard

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// shortTemporaryDirectory is TMPDIR for a test, short enough that a socket in it fits a unix
// socket's path, which t.TempDir's test-named directory may not.
func shortTemporaryDirectory(t *testing.T) string {
	dir, err := os.MkdirTemp("", "nvd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("TMPDIR", dir)
	return dir
}

// A program in a window named for the subject finds nvim's socket from it, a second diff of a
// subject whose nvim still listens is refused, and Ended reads each unopened tab from the file
// nvim's script writes, then removes every file Write wrote.
func TestNvimDiffNamesItsSocketFromTheSubjectAndEndedReadsWhatTheScriptLeft(t *testing.T) {
	tmp := shortTemporaryDirectory(t)
	diff := NvimDiff{
		Subject: "widget #13", Document: "# Count cache hits\n",
		Tabs: []NvimDiffTab{{Label: "widget", Checkout: "/src/widget", Revisions: "origin/main...refs/remotes/origin/pr/13"}},
	}
	h, err := diff.Write()
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(tmp, "prs-diff-widget-13.sock")
	if argv := h.Argv(); len(argv) != 5 || argv[1] != "--listen" || argv[2] != socket || argv[3] != "-S" || h.Dir() != "/src/widget" {
		t.Errorf("nvim runs %q in %s, want it listening on %s in /src/widget", argv, h.Dir(), socket)
	}
	script, _ := os.ReadFile(h.Argv()[4])
	if !strings.Contains(string(script), "'origin/main...refs/remotes/origin/pr/13'") {
		t.Errorf("the script opens no tab of the diff:\n%s", script)
	}

	nvim, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := diff.Write(); err == nil || !strings.Contains(err.Error(), "already open") {
		t.Errorf("a second diff of widget #13 while its nvim listens wrote %v, want it refused", err)
	}
	_ = nvim.Close()

	if err := os.WriteFile(h.file("unopened"), []byte("widget\tVim:E492: Not an editor command\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ending := h.Ended(NvimDiffNoTab)
	if want := []NvimDiffUnopened{{Label: "widget", Why: "Vim:E492: Not an editor command"}}; !slices.Equal(ending.Unopened, want) ||
		!strings.Contains(ending.NoDiff, "widget #13") || !strings.Contains(ending.NoDiff, "Vim:E492") {
		t.Errorf("Ended read %+v", ending)
	}
	if _, err := os.Stat(h.files); !os.IsNotExist(err) {
		t.Errorf("Ended left %s", h.files)
	}
}

// An nvim config reads the threads file by the schema it cites, so each kind Write writes holds
// to it, a thread whose comments were never read included.
func TestNvimDiffWritesEachKindsThreadsFileInItsSchema(t *testing.T) {
	shortTemporaryDirectory(t)
	compiler := jsonschema.NewCompiler()
	schema, err := jsonschema.UnmarshalJSON(bytes.NewReader(NvimDiffThreadsSchema))
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("nvim-diff-threads.schema.json", schema); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("nvim-diff-threads.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	line := 7
	pullRequest := NvimDiffThreads{
		Title: "Count cache hits", Slug: "owner/widget", Number: 13, URL: "https://github.com/owner/widget/pull/13",
		Base: "origin/main", Head: "abc123", ThreadsRead: true,
		Threads: []NvimDiffThread{{Path: "cache.go", Line: &line, Side: "RIGHT", CommentCount: 2}},
	}
	whole, sinceReview := pullRequest, pullRequest
	whole.Diff, sinceReview.Diff = NvimDiffWhole, NvimDiffSinceReview
	for _, threads := range []NvimDiffThreads{
		whole, sinceReview,
		{Diff: NvimDiffSession, Title: "session 8c5cbcad", Sessions: []string{"8c5cbcad-e9c9-4e43-b86c-41a243fb386e"}, ThreadsRead: true},
	} {
		h, err := NvimDiff{
			Subject: threads.Title, Document: "# " + threads.Title + "\n", Threads: threads,
			Tabs: []NvimDiffTab{{Label: "widget", Checkout: "/src/widget", Revisions: "a..b"}},
		}.Write()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(h.file("threads.json"))
		if err != nil {
			t.Fatal(err)
		}
		var written any
		if err := json.Unmarshal(raw, &written); err != nil {
			t.Fatal(err)
		}
		if err := compiled.Validate(written); err != nil {
			t.Errorf("the %s threads file breaks its schema: %v\n%s", threads.Diff, err, raw)
		}
		h.Ended(0)
	}
}
