package breadcrumb

import (
	"slices"
	"testing"
)

func TestFitCutsTheWidestThenDropsTheOutermost(t *testing.T) {
	for _, tc := range []struct {
		scene   string
		widths  []int
		room    int
		dropped int
		fitted  []int
	}{
		{"a long section name gives way while the command names stay whole", []int{2, 3, 30}, 30, 0, []int{2, 3, 19}},
		{"every segment at the floor, so the tool's name goes", []int{20, 20, 20}, 27, 1, []int{12, 12}},
		{"one segment wider than the room is cut to it", []int{40}, 25, 0, []int{25}},
	} {
		dropped, fitted := Fit(tc.widths, 3, tc.room)
		if dropped != tc.dropped || !slices.Equal(fitted, tc.fitted) {
			t.Errorf("%s: Fit = %d, %v, want %d, %v", tc.scene, dropped, fitted, tc.dropped, tc.fitted)
		}
	}
}
