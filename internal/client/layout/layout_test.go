package layout

import "testing"

func sampleWindows() []Window {
	return []Window{
		{AppName: "Firefox", WindowTitle: "GitHub", X: 0, Y: 0, W: 1920, H: 1080},
		{AppName: "Code", WindowTitle: "main.go", X: 100, Y: 50, W: 1280, H: 800},
		{AppName: "Terminal", WindowTitle: "claude", X: 200, Y: 200, W: 800, H: 600},
	}
}

// TestHashWindows: identical window sets in different orders produce
// identical hex digests.
func TestHashWindows(t *testing.T) {
	a := sampleWindows()
	b := []Window{a[2], a[0], a[1]}

	hashA := HashWindows(a)
	hashB := HashWindows(b)

	if hashA == "" {
		t.Fatal("HashWindows() returned empty digest")
	}
	if hashA != hashB {
		t.Errorf("hash differs across orderings:\n  A=%s\n  B=%s", hashA, hashB)
	}
}

// TestHashChange_AppName: changing one window's AppName flips the hash.
func TestHashChange_AppName(t *testing.T) {
	base := sampleWindows()
	mutated := sampleWindows()
	mutated[0].AppName = "Chromium"

	if HashWindows(base) == HashWindows(mutated) {
		t.Error("hash unchanged after AppName change")
	}
}

// TestHashChange_Title: changing one window's WindowTitle flips the hash.
func TestHashChange_Title(t *testing.T) {
	base := sampleWindows()
	mutated := sampleWindows()
	mutated[1].WindowTitle = "main_test.go"

	if HashWindows(base) == HashWindows(mutated) {
		t.Error("hash unchanged after WindowTitle change")
	}
}

// TestHashChange_Geometry: changing X, Y, W, or H individually each flips
// the hash.
func TestHashChange_Geometry(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Window)
	}{
		{"X", func(w *Window) { w.X++ }},
		{"Y", func(w *Window) { w.Y++ }},
		{"W", func(w *Window) { w.W++ }},
		{"H", func(w *Window) { w.H++ }},
	}
	base := sampleWindows()
	baseHash := HashWindows(base)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := sampleWindows()
			tc.mut(&mutated[0])
			if HashWindows(mutated) == baseHash {
				t.Errorf("hash unchanged after %s change", tc.name)
			}
		})
	}
}

// TestHashChange_Cardinality: adding or removing a window flips the hash.
func TestHashChange_Cardinality(t *testing.T) {
	base := sampleWindows()
	baseHash := HashWindows(base)

	added := append(sampleWindows(), Window{AppName: "Notes", WindowTitle: "scratch", X: 0, Y: 0, W: 400, H: 300})
	if HashWindows(added) == baseHash {
		t.Error("hash unchanged after adding a window")
	}

	removed := base[:2]
	if HashWindows(removed) == baseHash {
		t.Error("hash unchanged after removing a window")
	}
}

// TestHashEmpty: hashing nil and []Window{} returns the same digest, and
// both are well-defined (non-empty hex strings).
func TestHashEmpty(t *testing.T) {
	hashNil := HashWindows(nil)
	hashEmpty := HashWindows([]Window{})

	if hashNil == "" {
		t.Fatal("HashWindows(nil) returned empty digest")
	}
	if hashNil != hashEmpty {
		t.Errorf("nil and empty slice hash differently:\n  nil=%s\n  []=%s", hashNil, hashEmpty)
	}

	// And the empty-set digest must differ from a non-empty digest.
	if hashNil == HashWindows(sampleWindows()) {
		t.Error("empty-set hash collides with non-empty hash")
	}
}

// TestHashWindows_DefensiveCopy: HashWindows must not mutate the input
// slice (callers may iterate the slice for other purposes after hashing).
func TestHashWindows_DefensiveCopy(t *testing.T) {
	in := sampleWindows()
	first := in[0]
	_ = HashWindows(in)
	if in[0] != first {
		t.Errorf("input slice was reordered/mutated: in[0]=%+v want %+v", in[0], first)
	}
}
