package gotermboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// rememberedFile is what RemembersDisplayIn keeps: the tab showing when the reader last moved
// between tabs, and each tab's display settings by its name.
type rememberedFile struct {
	Showing string                    `json:"showing,omitempty"`
	Tabs    map[string]rememberedView `json:"tabs,omitempty"`
}

// rememberedView is one tab's display settings as the reader last set them. A setting the reader
// never changed is left out, so the board's own default holds for it.
type rememberedView struct {
	PanelOff  *bool           `json:"panel_off,omitempty"`
	Summaries *bool           `json:"summaries,omitempty"`
	Grouping  string          `json:"grouping,omitempty"`
	RawRows   *bool           `json:"raw_rows,omitempty"`
	Folded    map[string]bool `json:"folded,omitempty"`
	Opened    map[string]bool `json:"opened,omitempty"`
}

// rememberedDisplays holds what this program remembers and writes it to path.
type rememberedDisplays struct {
	path string
	// mu covers every field below, and a write holds it throughout, so each write carries every
	// change made before it.
	mu   sync.Mutex
	file rememberedFile
	// changedTabs and showingChanged are what this program changed. A write lays only them over
	// what the file holds then, so another program on the same file keeps what it set.
	changedTabs    map[string]bool
	showingChanged bool
	unwritten      bool
}

// RemembersDisplayIn keeps the program's display in the file at path, and opens on what the
// file holds. Each tab, by its name, keeps its side panel (p), its summaries under rows (f), its
// grouping (v), whether it shows the raw rows (RawRowsKey), and each group folded and row opened
// by enter, space or the board's own keys. A fold opened is not kept, since it lasts only while
// its rows sit together.
// The program opens on the tab the reader last moved to, unless NewTabs was told one to open
// on. A setting never changed keeps the board's default, and a remembered grouping the board
// does not offer opens on its first. Each change writes the whole file again. A file that will
// not read is said on the footer, and the next change writes over it.
func (t *Tabs) RemembersDisplayIn(path string) {
	file, err := readRememberedFile(path)
	t.memory = &rememberedDisplays{path: path, file: file, changedTabs: map[string]bool{}}
	for i, model := range t.models {
		model.memory, model.tabName = t.memory, t.names[i]
		model.recall(file.Tabs[t.names[i]])
	}
	if at := slices.Index(t.names, file.Showing); at >= 0 && !t.openNamed {
		t.at, t.reopens = at, at
	}
	if err != nil {
		t.active().said = err.Error()
	}
}

func readRememberedFile(path string) (rememberedFile, error) {
	var file rememberedFile
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &file)
	}
	if file.Tabs == nil {
		file.Tabs = map[string]rememberedView{}
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return rememberedFile{Tabs: map[string]rememberedView{}}, fmt.Errorf("every tab opens on its board's defaults, because %s will not read: %w", path, err)
	}
	return file, nil
}

// recall opens the model on the settings a past run remembered for its tab.
func (m *Model) recall(view rememberedView) {
	if view.PanelOff != nil {
		m.panelOff = *view.PanelOff
	}
	if view.Summaries != nil {
		m.display.under = *view.Summaries
	}
	if at := slices.Index(m.board.Groupings(), view.Grouping); at >= 0 && view.Grouping != "" {
		m.grouping = at
	}
	if view.RawRows != nil {
		m.display.showRaw(*view.RawRows)
	}
	for key, folded := range view.Folded {
		m.display.fold(key, folded)
	}
	for key, opened := range view.Opened {
		m.display.open(key, opened)
	}
}

// remember applies change to this tab's remembered settings, where its Tabs remembers them.
// Tabs writes them once the message that changed them is handled.
func (m *Model) remember(change func(*rememberedView)) {
	if m.memory == nil {
		return
	}
	m.memory.mu.Lock()
	defer m.memory.mu.Unlock()
	view := m.memory.file.Tabs[m.tabName]
	change(&view)
	m.memory.file.Tabs[m.tabName] = view
	m.memory.changedTabs[m.tabName] = true
	m.memory.unwritten = true
}

// rememberFolded and rememberOpened keep a group fold and a row open the reader chose. A jump
// unfolding a group to reach its row is not one.
func (m *Model) rememberFolded(key string, folded bool) {
	m.remember(func(view *rememberedView) { view.Folded = setKey(view.Folded, key, folded) })
}

func (m *Model) rememberOpened(key string, opened bool) {
	m.remember(func(view *rememberedView) { view.Opened = setKey(view.Opened, key, opened) })
}

func setKey(set map[string]bool, key string, on bool) map[string]bool {
	if set == nil {
		set = map[string]bool{}
	}
	set[key] = on
	return set
}

// showing remembers the tab the reader moved to.
func (r *rememberedDisplays) showing(name string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.file.Showing, r.showingChanged, r.unwritten = name, true, true
}

// rememberFault is a write of the remembered display that failed, for the model showing when it
// started.
type rememberFault struct {
	model *Model
	err   error
}

// written is a write of every change not yet written, or nil where there is none. A fault is
// said on the footer of model.
func (r *rememberedDisplays) written(model *Model) tea.Cmd {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	unwritten := r.unwritten
	r.unwritten = false
	r.mu.Unlock()
	if !unwritten {
		return nil
	}
	return func() tea.Msg {
		if err := r.write(); err != nil {
			return rememberFault{model: model, err: err}
		}
		return nil
	}
}

// write lays this program's changes over the file's and renames the result into place, so a
// reader never sees half a file. A file that will not read is written over.
func (r *rememberedDisplays) write() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, _ := readRememberedFile(r.path)
	for name := range r.changedTabs {
		file.Tabs[name] = r.file.Tabs[name]
	}
	if r.showingChanged {
		file.Showing = r.file.Showing
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(r.path)+".*")
	if err != nil {
		return err
	}
	_, writeErr := temp.Write(append(data, '\n'))
	err = errors.Join(writeErr, temp.Close())
	if err == nil {
		err = os.Rename(temp.Name(), r.path)
	}
	if err != nil {
		return errors.Join(err, os.Remove(temp.Name()))
	}
	return nil
}
