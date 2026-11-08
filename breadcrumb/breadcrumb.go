// Package breadcrumb fits a path of names, such as "uv › add › Options", on one
// line. It works on widths alone, so a caller draws the segments in its own
// colors, and it imports nothing that draws.
package breadcrumb

// segmentFloor is how narrow the widest segment is cut before the outermost
// goes altogether.
const segmentFloor = 12

// Fit says how wide each segment may be drawn for the path to fit room, with
// gap cells between neighbors. The widest is cut first, down to a floor, so a
// long title keeps its first words and every short segment stays whole. The
// outermost goes after that, and dropped counts how many went. The innermost
// always stays, cut to room where it is wider.
func Fit(widths []int, gap, room int) (dropped int, fitted []int) {
	fitted = append([]int(nil), widths...)
	total := -gap
	for _, w := range fitted {
		total += w + gap
	}
	for total > room && len(fitted) > 1 {
		widest := 0
		for i, w := range fitted {
			if w > fitted[widest] {
				widest = i
			}
		}
		if fitted[widest] > segmentFloor {
			fitted[widest]--
			total--
			continue
		}
		total -= fitted[0] + gap
		fitted = fitted[1:]
		dropped++
	}
	if len(fitted) == 1 {
		fitted[0] = min(fitted[0], max(room, 0))
	}
	return dropped, fitted
}
