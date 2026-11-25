// Package keywrites runs the writes a tab's keys start: off the screen's loop, queued in the order
// the keys started them and run one at a time, on the strip while they run and on the footer once
// each ends, and awaited by quitting. It keeps each row a write is pending on until a read begun
// after the write shows it, so a tab refuses a second key on that row.
package keywrites

import (
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermboard"
)

// Write is one write a key started. Row is the key of the one row the write acts on, which then
// stays pending, or empty for a write that reaches no one row.
type Write struct {
	Doing string
	Run   func() (string, error)
	Row   string
}

// Ended is how one write ended: what its Run answered, or why it did not land.
type Ended struct {
	Said   string
	Failed bool
}

// Pending is a row's newest write, from the key that queued it until a read begun after it ended
// lands, or for good where it failed. Running says it left the queue. Ended is when it landed, and
// Fault why it failed.
type Pending struct {
	Doing   string
	Running bool
	Ended   time.Time
	Fault   string
}

// Refusal is why a key on the row refuses while its write is pending, or empty where it does not. A
// failed write refuses nothing, so the row takes its next key.
func (p Pending) Refusal() string {
	switch {
	case p.Fault != "":
		return ""
	case p.Ended.IsZero():
		return p.Doing + " is still under way, so nothing was asked"
	}
	return p.Doing + " landed and the board is reading it again, so nothing was asked"
}

// Writes is a tab's queue of writes. One runs at a time, because two on one row would each act on
// what the other moved. A key never waits on the queue, so the next key starts at once.
type Writes struct {
	mu sync.Mutex
	// queue is every write started and not yet run, in the order the keys started them.
	queue []Write
	// doing and started are the write running, while one is.
	doing   string
	started time.Time
	// draining says a drain runs the queue, which a write started meanwhile joins rather than
	// starting another. idle closes when that drain finds the queue empty.
	draining bool
	idle     chan struct{}
	reread   func()
	pending  map[string]Pending
	// The newest write that ended. unsaid is its answer the footer has not said yet.
	ended  time.Time
	said   string
	failed bool
	unsaid bool
	// awaited collects every write that ends while quitting waits.
	awaiting bool
	awaited  []Ended
	// footer is the screen the newest write began on, which says each write's whole answer.
	footer *gotermboard.Model
}

func (w *Writes) underWay() []string {
	var doing []string
	if !w.started.IsZero() {
		doing = append(doing, w.doing)
	}
	for _, q := range w.queue {
		doing = append(doing, q.Doing)
	}
	return doing
}

// Start queues write, and returns the drain that runs the queue where none runs yet. A drain
// already running takes the write once those ahead of it end, so Start then returns nil.
func (w *Writes) Start(m *gotermboard.Model, write Write, reread func()) tea.Cmd {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.footer, w.reread = m, reread
	w.queue = append(w.queue, write)
	if write.Row != "" {
		if w.pending == nil {
			w.pending = map[string]Pending{}
		}
		w.pending[write.Row] = Pending{Doing: write.Doing}
	}
	if w.draining {
		return nil
	}
	w.draining = true
	w.idle = make(chan struct{})
	return w.drain
}

// drain runs the queue's writes in order until it is empty, then reads again once, so a run of
// decisions costs one read rather than one each. The read runs after the drain lets go of the
// queue, so a key pressed during it starts its write at once, and quitting never waits on it.
// Each write's call carries its own deadline.
func (w *Writes) drain() tea.Msg {
	for {
		write, ok := w.next(time.Now())
		if !ok {
			break
		}
		said, err := write.Run()
		w.end(write, said, err, time.Now())
	}
	w.mu.Lock()
	reread := w.reread
	w.mu.Unlock()
	if reread != nil {
		reread()
	}
	return nil
}

// next takes the queue's first write as the one running, or ends the drain where none is left.
func (w *Writes) next(now time.Time) (Write, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) == 0 {
		w.draining = false
		close(w.idle)
		return Write{}, false
	}
	write := w.queue[0]
	w.queue = w.queue[1:]
	w.doing, w.started = write.Doing, now
	if p, ok := w.pending[write.Row]; ok {
		p.Running = true
		w.pending[write.Row] = p
	}
	return write, true
}

// end records how write ended, as the newest write's answer and on its row.
func (w *Writes) end(write Write, said string, err error, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.doing, w.started = "", time.Time{}
	w.ended, w.said, w.failed, w.unsaid = now, said, err != nil, true
	if err != nil {
		w.said = err.Error()
	}
	if p, ok := w.pending[write.Row]; ok {
		p.Running, p.Ended = false, now
		if err != nil {
			p.Fault = err.Error()
		}
		w.pending[write.Row] = p
	}
	if w.awaiting {
		w.awaited = append(w.awaited, Ended{Said: w.said, Failed: w.failed})
	}
}

// Pending is row's pending write, if it has one.
func (w *Writes) Pending(row string) (Pending, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.pending[row]
	return p, ok
}

// PendingRows is every row a write is pending on, by its key.
func (w *Writes) PendingRows() map[string]Pending {
	w.mu.Lock()
	defer w.mu.Unlock()
	rows := make(map[string]Pending, len(w.pending))
	for key, p := range w.pending {
		rows[key] = p
	}
	return rows
}

// Settle lets go of every row whose write landed before a read that began at readFrom, because
// that read shows what the write did. A failed row stays until the next write on it.
func (w *Writes) Settle(readFrom time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for key, p := range w.pending {
		if p.Fault == "" && !p.Ended.IsZero() && p.Ended.Before(readFrom) {
			delete(w.pending, key)
		}
	}
}

// Writing says a write runs or waits in the queue, without waiting for it.
func (w *Writes) Writing() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.underWay()) > 0
}

// Await waits for every write running or queued to end, then answers how each ended, in order.
// announce hears what it waits for before the wait starts, in the same step that finds them, so no
// write ends unheard between the two. Nothing under way returns nil and announces nothing. Quitting
// calls it, so the process never exits under a write half done or one a key queued and nothing ran.
func (w *Writes) Await(announce func(doing []string)) []Ended {
	w.mu.Lock()
	doing := w.underWay()
	if len(doing) == 0 {
		w.mu.Unlock()
		return nil
	}
	idle := w.idle
	w.awaiting, w.awaited = true, nil
	w.mu.Unlock()
	if announce != nil {
		announce(doing)
	}
	<-idle
	w.mu.Lock()
	defer w.mu.Unlock()
	w.awaiting = false
	return w.awaited
}

// saidFor is how long a write that worked stays on the strip. One refused stays until the next,
// because nothing else says it did not land.
const saidFor = time.Minute

// overdueAfter is how long a write runs before the strip draws it in red, because a service that
// stops answering holds every write queued behind it and the quit that waits for them.
const overdueAfter = 2 * time.Minute

// Line is the strip's line about the writes: what runs and for how long in yellow, red once it is
// overdue, with how many wait behind it, then what the newest did, for saidFor, or why it was
// refused in red. The footer says the whole answer when a write ends, so here an answer of several
// lines reads as one, and the screen's edge cuts it.
func (w *Writes) Line(now time.Time) gotermboard.Line {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case !w.started.IsZero():
		code := gotermboard.Yellow
		if now.Sub(w.started) > overdueAfter {
			code = gotermboard.Red
		}
		line := gotermboard.Line{gotermboard.Ticking(w.doing+" "+gotermboard.Ago(now.Sub(w.started)), code)}
		if n := len(w.queue); n > 0 {
			line = append(line, gotermboard.C(", then "+strconv.Itoa(n)+" more queued", code))
		}
		return line
	case w.ended.IsZero():
		return nil
	case w.failed:
		return gotermboard.Line{gotermboard.C(oneLine(w.said), gotermboard.Red)}
	case now.Sub(w.ended) > saidFor:
		return nil
	}
	return gotermboard.Line{gotermboard.C(oneLine(w.said), "")}
}

func oneLine(text string) string {
	return strings.ReplaceAll(strings.TrimSpace(text), "\n", "; ")
}

// SayEnded puts the newest ended write's whole answer on the footer, wrapped, which the strip's one
// line cuts. It runs as the frame is built, on the screen's loop.
func (w *Writes) SayEnded() {
	w.mu.Lock()
	said, ended, footer := w.said, w.unsaid, w.footer
	w.unsaid = false
	w.mu.Unlock()
	if ended && footer != nil {
		footer.Say(said)
	}
}
