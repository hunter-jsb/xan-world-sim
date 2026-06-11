package main

import "testing"

func TestPOIOrderAndWrap(t *testing.T) {
	pois := []poi{
		{X: 5, Y: 1, Name: "A", Kind: "seat"},
		{X: 2, Y: 3, Name: "B", Kind: "lake"},
		{X: 9, Y: 3, Name: "C", Kind: "den"},
		{X: 1, Y: 7, Name: "D", Kind: "pass"},
	}
	cases := []struct {
		x, y          int64
		after, before string
	}{
		{0, 0, "A", "D"},  // before everything: w→first, b wraps to last
		{5, 1, "B", "D"},  // exactly on A: strict ordering moves off it
		{2, 3, "C", "A"},  // on B
		{9, 3, "D", "B"},  // on C
		{1, 7, "A", "C"},  // on D: w wraps to first
		{50, 9, "A", "D"}, // past everything: w wraps, b→last
		{4, 3, "C", "B"},  // between B and C on the same row
	}
	for _, c := range cases {
		if i := poiAfter(pois, c.x, c.y); pois[i].Name != c.after {
			t.Errorf("poiAfter(%d,%d) = %s, want %s", c.x, c.y, pois[i].Name, c.after)
		}
		if i := poiBefore(pois, c.x, c.y); pois[i].Name != c.before {
			t.Errorf("poiBefore(%d,%d) = %s, want %s", c.x, c.y, pois[i].Name, c.before)
		}
	}
}

func TestPOIEmpty(t *testing.T) {
	if i := poiAfter(nil, 0, 0); i != -1 {
		t.Errorf("poiAfter(empty) = %d, want -1", i)
	}
	if i := poiBefore(nil, 0, 0); i != -1 {
		t.Errorf("poiBefore(empty) = %d, want -1", i)
	}
}
