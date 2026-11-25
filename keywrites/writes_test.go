package keywrites

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Two keys write on two rows before the first write ends, and the second is refused. The second
// joins the drain the first started, each row refuses a key while its write is pending, quitting
// waits for both and hears them in order, one read follows the drain, and that read releases the
// landed row while the refused one stays with its error and takes the next key.
func TestWritesRunInOrderQuittingAwaitsThemAndARefusedRowKeepsItsError(t *testing.T) {
	var w Writes
	release := make(chan struct{})
	var rereads atomic.Int32
	reread := func() { rereads.Add(1) }
	write := func(row string, err error) Write {
		return Write{Doing: "writing " + row, Row: row, Run: func() (string, error) {
			<-release
			if err != nil {
				return "", err
			}
			return row + " written", nil
		}}
	}

	drain := w.Start(nil, write("a", nil), reread)
	if drain == nil {
		t.Fatal("the first write started no drain")
	}
	if joined := w.Start(nil, write("b", errors.New("b was refused")), reread); joined != nil {
		t.Fatal("the second write started a drain of its own while the first waited")
	}
	if p, _ := w.Pending("a"); !strings.Contains(p.Refusal(), "still under way") || !w.Writing() {
		t.Fatalf("before the drain runs, a's refusal is %q and Writing is %v", p.Refusal(), w.Writing())
	}

	go drain()
	announced := make(chan []string, 1)
	quit := make(chan []Ended)
	go func() { quit <- w.Await(func(doing []string) { announced <- doing }) }()
	if doing := <-announced; !slices.Equal(doing, []string{"writing a", "writing b"}) {
		t.Fatalf("quitting announced %v, want both writes in order", doing)
	}
	close(release)
	want := []Ended{{Said: "a written"}, {Said: "b was refused", Failed: true}}
	if ended := <-quit; !slices.Equal(ended, want) {
		t.Fatalf("quitting heard %+v, want %+v", ended, want)
	}
	for deadline := time.Now().Add(5 * time.Second); rereads.Load() == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no read followed the drain")
		}
	}

	if p, _ := w.Pending("a"); !strings.Contains(p.Refusal(), "landed") {
		t.Fatalf("before the next read, a's refusal is %q, want that it landed", p.Refusal())
	}
	w.Settle(time.Now().Add(time.Second))
	rows := w.PendingRows()
	if _, held := rows["a"]; held {
		t.Error("a read begun after a landed still holds a pending")
	}
	if b := rows["b"]; b.Fault != "b was refused" || b.Refusal() != "" {
		t.Errorf("b holds %+v with refusal %q, want its error and no refusal", b, b.Refusal())
	}
}
