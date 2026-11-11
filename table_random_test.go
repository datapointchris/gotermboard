package gotermboard

import (
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/datapointchris/gotermstyle"
)

// randomBoard is a board whose frame a seed makes, over every field a Row, a Group and a Look has.
type randomBoard struct {
	rng       *rand.Rand
	frame     Frame
	look      Look
	tails     bool
	twoLines  bool
	revision  uint64
	groupings []string
	// numbered says the board sets Revision. Its twin leaves it zero and must draw the same.
	numbered bool
	// ownKeys gives every row a key no other row holds. Two rows of one group that share a key share
	// one change cue, which a frame laid out again lights on every frame and a kept frame lights once.
	ownKeys bool
	text    map[string]string
	// frames counts the frames built. The strip and the charts carry it, so a model that kept an
	// old frame's strip or charts draws a different screen from one that took the new ones.
	frames int
}

func (b *randomBoard) Sources() []Refresher { return nil }
func (b *randomBoard) Groupings() []string  { return b.groupings }
func (b *randomBoard) Tails() bool          { return b.tails }
func (b *randomBoard) TwoLines() bool       { return b.twoLines }
func (b *randomBoard) Look(int) Look        { return b.look }

func (b *randomBoard) Frame(_ time.Time, grouping string) Frame {
	frame := b.frame
	if grouping == "two" {
		frame.Groups = frame.Groups[:max(len(frame.Groups)/2, 1)]
	}
	if b.numbered {
		frame.Revision = b.revision
	}
	b.frames++
	count := strconv.Itoa(b.frames)
	if len(frame.Strip) > 0 {
		frame.Strip = []Line{append(slices.Clone(frame.Strip[0]), C(" frame "+count, ""))}
	}
	if frame.Charts != nil {
		frame.Charts = func(width, _ int) []Line {
			return []Line{{C(strings.Repeat("▁", min(width, 30))+" "+count, Cyan)}}
		}
	}
	return frame
}

// Receive is ReceivingBoard's: a message changes the text a row's document reads, and rebuilds.
func (b *randomBoard) Receive(m *Model, msg tea.Msg) tea.Cmd {
	if said, ok := msg.(randomSaid); ok {
		b.text[said.key] = said.text
		b.revision++
		m.Rebuild()
	}
	return nil
}

type randomSaid struct{ key, text string }

var randomWords = []string{
	"queue", "a passage to rewrite", "日本語のテキスト", "λόγος", "🚀 launched", "cache write 139k", "x",
	"the quick brown fox jumps over the lazy dog and keeps running past the screen's edge", "é́ combining", "",
}

func (b *randomBoard) words() string { return randomWords[b.rng.IntN(len(randomWords))] }

func (b *randomBoard) often(in int) bool { return b.rng.IntN(in) == 0 }

func (b *randomBoard) code() string {
	return []string{"", Red, Green, Cyan, BoldYellow, Bold}[b.rng.IntN(6)]
}

func (b *randomBoard) line(cells int) Line {
	line := make(Line, 0, cells)
	for range cells {
		line = append(line, C(b.words()+" ", b.code()))
	}
	return line
}

func (b *randomBoard) document(key string) Document {
	paragraphs := 1 + b.rng.IntN(6)
	if b.often(8) {
		paragraphs = 0
	}
	sentence := strings.Repeat(b.words()+" ", 1+b.rng.IntN(12))
	return func(width int) []Line {
		var lines []Line
		if said := b.text[key]; said != "" {
			lines = append(lines, Line{C(said, Bold)})
		}
		for range paragraphs {
			for _, piece := range wrap(sentence, max(width, 10)) {
				lines = append(lines, Line{C(piece, "")})
			}
		}
		return lines
	}
}

func (b *randomBoard) row(key string, columns int) Row {
	r := Row{Key: key, Text: b.words()}
	if b.often(6) {
		r.Span = b.line(1 + b.rng.IntN(3))
	} else {
		for range columns {
			r.Cells = append(r.Cells, C(b.words(), b.code()))
		}
		if b.often(12) {
			r.Cells[0] = Ticking(b.words(), "")
		}
	}
	if b.often(5) {
		r.Symbol = C("●", Green)
	}
	if b.often(5) && len(r.Cells) > 0 {
		r.WrapLast = TallyBar(b.rng.IntN(40), b.rng.IntN(40), b.rng.IntN(5), b.rng.IntN(60))
	}
	if b.often(2) {
		r.Under = C(b.words()+" "+b.words(), b.code())
		r.UnderWhereCut = b.often(4)
	}
	if b.often(4) && len(r.Cells) > 0 {
		r.Second = b.line(1 + b.rng.IntN(3))
	}
	switch b.rng.IntN(4) {
	case 0:
		r.Detail = []Cell{C(strings.Repeat(b.words()+" ", 1+b.rng.IntN(20)), ""), C(b.words(), Cyan)}
	case 1:
		r.Document = b.document(key)
	}
	r.StartsOpen = b.often(4)
	r.Marked = b.often(9)
	if b.often(7) {
		r.Panel = b.document(key + "/panel")
	}
	if b.often(6) {
		r.ReaderTitle = b.words()
	}
	return r
}

func (b *randomBoard) group(key string, depth, columns int, keys *[]string) Group {
	g := Group{Key: key, Title: Line{C("group "+key, b.code())}, Counted: b.often(2), Count: b.rng.IntN(90), Text: b.words()}
	if b.often(4) {
		for range columns - 1 {
			g.Cells = append(g.Cells, C(b.words(), b.code()))
		}
	}
	if b.often(6) {
		g.WrapLast = TallyBar(b.rng.IntN(60), b.rng.IntN(30), 1, b.rng.IntN(80))
	}
	if b.often(4) {
		g.Below = append(g.Below, b.line(2), Items(Cyan, "one", "two", "three", b.words(), "five", "six"))
	}
	if b.often(5) {
		g.Span = b.line(1 + b.rng.IntN(6))
		if b.often(2) {
			g.Beside = Items(Yellow, "left", "right", b.words())
		}
	}
	if b.often(4) {
		g.Under = C(strings.Repeat(b.words()+" ", 1+b.rng.IntN(15)), "")
	}
	if b.often(8) {
		g.Document = b.document(key)
	}
	if b.often(3) {
		g.Summary = b.line(2)
	}
	g.Folded = b.often(6)
	switch b.rng.IntN(5) {
	case 0:
		g.Gap = true
	case 1:
		g.Rule, g.BlankAboveRule = true, b.often(2)
	}
	if b.often(5) {
		g.BetweenRows = []RuleWeight{RuleThin, RuleDotted, RuleDouble}[b.rng.IntN(3)]
	}
	if b.often(6) {
		g.Overline, g.Tone = RuleDouble, b.code()
	}
	if b.often(6) {
		g.Underline = RuleThin
	}
	g.Still = b.often(7)
	g.Heading = b.often(8)
	g.FilterOnly = b.often(14)
	if b.often(6) {
		g.Detail = []Cell{C(b.words(), "")}
	}
	for range b.rng.IntN(9) {
		rowKey := key + "/r" + strconv.Itoa(len(*keys))
		switch {
		case b.ownKeys:
		case b.often(25):
			rowKey = ""
		case b.often(20) && len(*keys) > 0:
			rowKey = (*keys)[b.rng.IntN(len(*keys))]
		}
		*keys = append(*keys, rowKey)
		g.Rows = append(g.Rows, b.row(rowKey, columns))
	}
	if depth < 3 {
		for i := range b.rng.IntN(3) {
			g.Groups = append(g.Groups, b.group(key+"/g"+strconv.Itoa(i), depth+1, columns, keys))
		}
	}
	return g
}

// build makes a frame, and says the keys of its rows.
func (b *randomBoard) build() []string {
	columns := 1 + b.rng.IntN(4)
	frame := Frame{}
	for i := range columns {
		frame.Columns = append(frame.Columns, Column{Title: []string{"NAME", "STATE", "", "WHAT"}[i], Right: b.often(5), Max: []int{0, 0, 30}[b.rng.IntN(3)]})
	}
	var keys []string
	for i := range 1 + b.rng.IntN(7) {
		frame.Groups = append(frame.Groups, b.group("g"+strconv.Itoa(i), 0, columns, &keys))
	}
	if b.often(3) {
		frame.Strip = []Line{b.line(2)}
	}
	if b.often(6) {
		frame.Charts = func(width, _ int) []Line { return []Line{{C(strings.Repeat("▁", min(width, 30)), Cyan)}} }
	}
	if b.often(5) && len(keys) > 0 {
		frame.StartAt = keys[b.rng.IntN(len(keys))]
	}
	b.frame = frame
	b.revision++
	return keys
}

func newRandomBoard(seed uint64, numbered, ownKeys bool) (*randomBoard, []string) {
	b := &randomBoard{rng: rand.New(rand.NewPCG(seed, 7)), numbered: numbered, ownKeys: ownKeys, text: map[string]string{}}
	if b.often(2) {
		b.look = Look{Sides: b.rng.IntN(3), Ends: b.rng.IntN(2), Spread: b.often(2), Rules: b.often(2), BoldCursor: b.often(2), NoRowChevrons: b.often(2)}
	}
	b.tails, b.twoLines = b.often(3), b.often(2)
	b.groupings = [][]string{nil, {"one", "two"}}[b.rng.IntN(2)]
	keys := b.build()
	return b, keys
}

var randomKeys = []tea.KeyPressMsg{
	downKey, downKey, downKey, upKey, upKey, typedKey("g"), typedKey("G"), typedKey("J"), typedKey("K"), typedKey("]"), typedKey("["),
	{Code: 'd', Mod: tea.ModCtrl},
	{Code: 'u', Mod: tea.ModCtrl},
	spaceKey, spaceKey, enterKey, typedKey("f"), typedKey("p"), typedKey("v"),
	typedKey("?"),
	{Code: tea.KeyEscape},
	typedKey("/"), typedKey("r"), typedKey("e"),
	{Code: tea.KeyBackspace},
	enterKey,
}

// randomStep does one thing to a model and its board. Every step reads its choices from rng alone,
// so two models stepped from equal seeds are sent the same things.
func randomStep(m *Model, b *randomBoard, keys []string, rng *rand.Rand, at time.Time) []string {
	switch roll := rng.IntN(100); {
	case roll < 60:
		m.Update(randomKeys[rng.IntN(len(randomKeys))])
	case roll < 68:
		m.Update(tea.WindowSizeMsg{Width: 40 + rng.IntN(180), Height: 8 + rng.IntN(50)})
	case roll < 76:
		m.Update(tickMsg(at))
	case roll < 82:
		keys = b.build()
		m.Update(tickMsg(at))
	case roll < 86:
		m.ScrollBy(rng.IntN(9) - 4)
	case roll < 89:
		m.ToggleEveryRow()
	case roll < 93:
		if len(keys) > 0 {
			m.MoveCursorTo(keys[rng.IntN(len(keys))])
		}
	case roll < 96:
		if len(keys) > 0 {
			m.Update(randomSaid{key: keys[rng.IntN(len(keys))], text: "said " + strconv.Itoa(rng.IntN(1000))})
		}
	case roll < 98:
		// A tab hides, is sometimes resized while hidden, and comes back.
		m.hide()
		if rng.IntN(2) == 0 {
			m.Update(tea.WindowSizeMsg{Width: 40 + rng.IntN(180), Height: 8 + rng.IntN(50)})
		}
		m.reveal(at)
	default:
		m.Rebuild()
	}
	return keys
}

func randomBoards(fallback int) int {
	if n, err := strconv.Atoi(os.Getenv("RANDOM_BOARDS")); err == nil {
		return n
	}
	return fallback
}

// TestTheCulledDrawMatchesTheWholeTableOnRandomBoards drives random boards through random keys,
// sizes, frames and messages. The hooks TestMain sets compare every draw, every follow and every
// reveal with the whole table's, and panic where one differs.
func TestTheCulledDrawMatchesTheWholeTableOnRandomBoards(t *testing.T) {
	before := held
	const steps = 80
	for seed := range uint64(randomBoards(20)) {
		b, keys := newRandomBoard(seed, seed%2 == 0, false)
		m := NewModel(b, gotermstyle.NewPalette(seed%3 != 0))
		m.Update(tea.WindowSizeMsg{Width: 60 + int(seed%5)*40, Height: 12 + int(seed%4)*12})
		rng := rand.New(rand.NewPCG(seed, 11))
		at := time.Now()
		for range steps {
			at = at.Add(time.Duration(rng.IntN(9)) * time.Second)
			keys = randomStep(m, b, keys, rng, at)
			m.View()
		}
	}
	t.Logf("held to the whole table: %d draws (%d scrolled, %d under a band, %d with pinned lines, %d beside a panel), %d follows, %d reveals, %d nodes",
		held.draws-before.draws, held.scrolled-before.scrolled, held.banded-before.banded, held.pinned-before.pinned, held.panels-before.panels,
		held.follows-before.follows, held.reveals-before.reveals, held.nodes-before.nodes)
	if drawHook != nil && (held.draws == before.draws || held.banded == before.banded || held.reveals == before.reveals) {
		t.Fatal("the random boards reached no draw, no band or no reveal, so the hooks held nothing")
	}
}

// TestABoardThatNumbersItsFramesDrawsWhatItsTwinDrawsEveryFrame steps two models alike. One board
// sets Revision and the other leaves it zero, so the first keeps its layout across the frames that
// did not change and the second lays every frame out. Every screen has to match, the change cue's
// coming and going with it.
func TestABoardThatNumbersItsFramesDrawsWhatItsTwinDrawsEveryFrame(t *testing.T) {
	const steps = 120
	screens := 0
	for seed := range uint64(randomBoards(20)) {
		numbered, keys := newRandomBoard(seed, true, true)
		plain, _ := newRandomBoard(seed, false, true)
		pal := gotermstyle.NewPalette(true)
		kept, laid := NewModel(numbered, pal), NewModel(plain, pal)
		size := tea.WindowSizeMsg{Width: 60 + int(seed%5)*40, Height: 12 + int(seed%4)*12}
		kept.Update(size)
		laid.Update(size)
		keptRNG, laidRNG := rand.New(rand.NewPCG(seed, 13)), rand.New(rand.NewPCG(seed, 13))
		at := time.Now()
		for step := range steps {
			at = at.Add(time.Duration(step%7) * time.Second)
			keys = randomStep(kept, numbered, keys, keptRNG, at)
			randomStep(laid, plain, keys, laidRNG, at)
			if got, want := kept.screen(), laid.screen(); got != want {
				gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
				for i := range min(len(gotLines), len(wantLines)) {
					if gotLines[i] != wantLines[i] {
						t.Fatalf("seed %d, step %d, line %d: the board that numbers its frames drew\n%q\nand its twin drew\n%q", seed, step, i, gotLines[i], wantLines[i])
					}
				}
				t.Fatalf("seed %d, step %d: %d lines against %d", seed, step, len(gotLines), len(wantLines))
			}
			screens++
		}
	}
	t.Logf("%d screens matched", screens)
}

// TestADrawPaintsTheNodesOnScreenHoweverManyTheBoardHolds counts the nodes painted. A draw paints
// no more nodes than the screen has lines, and a cursor key or a tick whose Revision held paints
// none ahead of its draw, on a small board and on a large one.
func TestADrawPaintsTheNodesOnScreenHoweverManyTheBoardHolds(t *testing.T) {
	hook, follow, reveal, draw := paintHook, followHook, revealHook, drawHook
	defer func() { paintHook, followHook, revealHook, drawHook = hook, follow, reveal, draw }()
	followHook, revealHook, drawHook = nil, nil, nil
	painted := 0
	paintHook = func(node, int, int) { painted++ }
	const height = 50
	for _, rows := range []int{500, 50000} {
		m, board := newLogModel(rows)
		board.revision = 1
		m.Update(tickMsg(time.Now()))
		m.View()
		frames := board.frames
		for name, step := range map[string]tea.Msg{"a cursor key": upKey, "a tick whose Revision held": tickMsg(time.Now()), "a half-screen key": tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}} {
			painted = 0
			m.Update(step)
			if painted != 0 {
				t.Errorf("%d rows: %s painted %d nodes ahead of its draw", rows, name, painted)
			}
			m.View()
			if painted == 0 || painted > height {
				t.Errorf("%d rows: the draw after %s painted %d nodes on a screen of %d lines", rows, name, painted, height)
			}
		}
		if got := m.layout.ix.total(); got < rows {
			t.Fatalf("%d rows laid out as %d lines", rows, got)
		}
		if board.frames != frames+1 {
			t.Errorf("%d rows: the board built %d frames for one tick", rows, board.frames-frames)
		}
	}
}
