package gotermboard

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Status is what a view says about one source, on its strip and in its --json.
type Status struct {
	Name    string    `json:"name"`
	Every   string    `json:"every"`
	ReadAt  time.Time `json:"read_at,omitzero"`
	Took    string    `json:"took,omitempty"`
	TriedAt time.Time `json:"tried_at,omitzero"`
	// Fault is why the newest try failed, and empty where it read.
	Fault string `json:"fault,omitempty"`
	// Note is what a read that succeeded could not cover, such as a host asleep.
	Note string `json:"note,omitempty"`
	// Reading says a read is under way.
	Reading bool `json:"reading,omitempty"`
	every   time.Duration
}

// Refresher is a Source with its value's type out of the way, for the loop that
// only starts reads.
type Refresher interface {
	// Claim reports whether a read is due at now, and marks one under way.
	Claim(now time.Time) bool
	Refresh(ctx context.Context)
	// Force makes the next Claim true.
	Force()
	Status() Status
}

// Source is one read at its own interval, holding its last good value.
type Source[T any] struct {
	name  string
	every time.Duration
	read  func(context.Context) (T, string, error)

	mu     sync.Mutex
	value  T
	status Status
	forced bool
}

// NewSource declares a read. read answers the value, a note on what it could not
// cover, and a failure, which leaves the last good value in place.
func NewSource[T any](name string, every time.Duration, read func(context.Context) (T, string, error)) *Source[T] {
	return &Source[T]{name: name, every: every, read: read, status: Status{Name: name, Every: every.String(), every: every}}
}

// Value is the last good read, and the zero value before the first.
func (s *Source[T]) Value() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

// Status is how the source stands now.
func (s *Source[T]) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Claim is Refresher's. A read still under way is never started twice, so a slow
// source falls behind rather than piling up.
func (s *Source[T]) Claim(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	due := s.forced || now.Sub(s.status.TriedAt) >= s.every
	if s.status.Reading || !due {
		return false
	}
	s.forced = false
	s.status.Reading, s.status.TriedAt = true, now
	return true
}

// Force is Refresher's.
func (s *Source[T]) Force() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forced = true
}

// Refresh reads once, keeping the last good value where the read fails.
func (s *Source[T]) Refresh(ctx context.Context) {
	started := time.Now()
	value, note, err := s.read(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Reading = false
	s.status.TriedAt = started
	s.status.Took = time.Since(started).Round(time.Millisecond).String()
	if err != nil {
		s.status.Fault = oneLine(err.Error())
		return
	}
	s.value = value
	s.status.ReadAt = time.Now()
	s.status.Fault, s.status.Note = "", note
}

// RefreshAll reads every source once, all at once, and returns when each ended.
func RefreshAll(ctx context.Context, sources ...Refresher) {
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Go(func() { source.Refresh(ctx) })
	}
	wg.Wait()
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:119] + "…"
	}
	return s
}

// Ago is a duration as one unit: 45s, 14m, 26h, 3d.
func Ago(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// lateAfter is how old a read gets before the strip names it, so a read landing a moment
// past its interval never flickers there.
func lateAfter(every time.Duration) time.Duration { return 2*every + 5*time.Second }

// late reports a read well past its interval, counting a first read from when it began.
func (s Status) late(now time.Time) bool {
	since := s.ReadAt
	if since.IsZero() {
		since = s.TriedAt
	}
	return !since.IsZero() && now.Sub(since) > lateAfter(s.every)
}

// ReadStatus is the one line a strip says about its reads. While every read is on
// schedule it says how old the newest is. A failed read is named in red with its reason,
// a late one in yellow, and a note follows plain, because a read that covered less still
// read. The screen's edge cuts them in that order.
func ReadStatus(statuses []Status, now time.Time) Line {
	if len(statuses) == 0 {
		return nil
	}
	line := Line{newestRead(statuses, now)}
	for _, cell := range failedCells(statuses, now) {
		line = append(line, C(" · ", ""), cell)
	}
	if cell, named := lateCell(statuses, now); named {
		line = append(line, C(" · ", ""), cell)
	}
	for _, cell := range noteCells(statuses) {
		line = append(line, C(" · ", ""), cell)
	}
	return line
}

func newestRead(statuses []Status, now time.Time) Cell {
	var newest time.Time
	for _, s := range statuses {
		if s.ReadAt.After(newest) {
			newest = s.ReadAt
		}
	}
	if newest.IsZero() {
		return C("reading…", "")
	}
	return Ticking("read "+Ago(now.Sub(newest))+" ago", "")
}

// failedCells is one cell per distinct failure, because a rollout fails several reads
// with one sentence. It names the reads it stopped, and the oldest good value among them.
func failedCells(statuses []Status, now time.Time) []Cell {
	var order []string
	failed := map[string][]Status{}
	for _, s := range statuses {
		if s.Fault == "" {
			continue
		}
		if _, seen := failed[s.Fault]; !seen {
			order = append(order, s.Fault)
		}
		failed[s.Fault] = append(failed[s.Fault], s)
	}
	cells := make([]Cell, 0, len(order))
	for _, fault := range order {
		cells = append(cells, C(namesAndAge(failed[fault], now)+": "+fault, Red))
	}
	return cells
}

func lateCell(statuses []Status, now time.Time) (Cell, bool) {
	var late []Status
	reading := false
	for _, s := range statuses {
		if s.Fault == "" && s.late(now) {
			late = append(late, s)
			reading = reading || s.Reading
		}
	}
	if len(late) == 0 {
		return Cell{}, false
	}
	text := namesAndAge(late, now)
	if reading {
		text += ", still reading"
	}
	return C(text, Yellow), true
}

// noteCells is one cell per distinct note, naming the reads that carry it.
func noteCells(statuses []Status) []Cell {
	var order []string
	named := map[string][]string{}
	for _, s := range statuses {
		if s.Note == "" {
			continue
		}
		if _, seen := named[s.Note]; !seen {
			order = append(order, s.Note)
		}
		named[s.Note] = append(named[s.Note], s.Name)
	}
	cells := make([]Cell, 0, len(order))
	for _, note := range order {
		cells = append(cells, C(strings.Join(named[note], ", ")+": "+note, ""))
	}
	return cells
}

// namesAndAge is reads' names and how old the oldest is, or unread if one has no value.
func namesAndAge(statuses []Status, now time.Time) string {
	names := make([]string, 0, len(statuses))
	var oldest time.Time
	unread := false
	for _, s := range statuses {
		names = append(names, s.Name)
		unread = unread || s.ReadAt.IsZero()
		if oldest.IsZero() || s.ReadAt.Before(oldest) {
			oldest = s.ReadAt
		}
	}
	if unread {
		return strings.Join(names, ", ") + " unread"
	}
	return strings.Join(names, ", ") + " " + Ago(now.Sub(oldest)) + " old"
}

// readCell is one read's age for the list of every read.
func readCell(s Status, now time.Time) Cell {
	age := Ago(now.Sub(s.ReadAt))
	switch {
	case s.Fault != "" && s.ReadAt.IsZero():
		return C("failed", Red)
	case s.Fault != "":
		return C(age+" old, failing", Red)
	case s.ReadAt.IsZero():
		return C("reading…", "")
	case s.late(now):
		return C(age+" ago", Yellow)
	}
	return C(age+" ago", "")
}
