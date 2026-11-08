package gotermboard

import (
	"slices"
	"testing"
)

func TestASectionSplitAcrossLayersHeadsTheOverlayOnceWhereItsFirstRowStands(t *testing.T) {
	own := []KeyAction[int]{
		{Keys: []string{"h"}, Section: SectionMoving},
		{Keys: []string{"C"}, Section: "On an item"},
	}
	universal := []KeyAction[int]{
		{Keys: []string{"j"}, Section: SectionMoving},
		{Keys: []string{"?"}, Section: SectionEverywhere},
		{Keys: []string{"J"}, Section: SectionMoving},
	}
	var got [][]string
	for _, section := range BySection(ActionsNow(0, own, universal)) {
		keys := []string{section.Section}
		for _, action := range section.Actions {
			keys = append(keys, action.Keys...)
		}
		got = append(got, keys)
	}
	want := [][]string{{SectionMoving, "h", "j", "J"}, {"On an item", "C"}, {SectionEverywhere, "?"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("the overlay's sections were %q, want %q", got, want)
	}
}
