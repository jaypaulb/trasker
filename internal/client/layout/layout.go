// Package layout captures and persists the set of open windows on the
// client desktop, syncs them to the trasker server, and exposes a minimal
// enumerator interface that each platform implements via build tags.
//
// Privacy: a Window holds exactly six fields — AppName, WindowTitle, X, Y,
// W, H — and nothing else. Process identifiers, command-line strings,
// executable paths, screen-capture bytes, focus state, stacking order, and
// monitor indices are all out of scope. Locked by Phase 7 decisions
// D-15/D-16 (privacy is a one-way door).
package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Window is a single open top-level window observed on the desktop.
//
// The struct is deliberately frozen at six fields. Adding any other field
// violates D-15/D-16 and is rejected by the package's privacy grep gate.
// See the package doc for the list of fields that are out of scope.
type Window struct {
	AppName     string `json:"app_name"`
	WindowTitle string `json:"window_title"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
}

// Snapshot is one captured row in the local layout_snapshots table.
type Snapshot struct {
	ID          int64
	CapturedAt  time.Time
	Windows     []Window
	WindowsHash string
	SyncedAt    *time.Time
	CreatedAt   time.Time
}

// Enumerator is the platform-specific window enumerator interface. The
// production implementation on Linux+cgo is X11Enumerator. Other platforms
// (Linux+nocgo, macOS, Windows) return ErrUnsupported from their
// constructor (NewPlatformEnumerator) and never satisfy this interface.
type Enumerator interface {
	// Enumerate returns the current set of open top-level windows.
	// Returns ErrUnsupported on platforms where enumeration is not
	// implemented for v1.
	Enumerate() ([]Window, error)
	// Close releases any platform resources (X11 display, etc.).
	Close() error
}

// ErrUnsupported is returned by NewPlatformEnumerator on platforms where
// layout enumeration is not implemented for v1 (CGO_ENABLED=0 Linux,
// macOS, Windows). The daemon logs a Warn and continues without layout
// capture.
var ErrUnsupported = errors.New("layout: window enumeration not supported on this build/platform")

// HashWindows returns the canonical SHA-256 hex digest of the given window
// set. The input slice is treated as an unordered set: identical sets in
// any order yield identical digests.
//
// Canonical form: sort by (AppName, WindowTitle, X, Y, W, H) ascending,
// then write "%s\x00%s\x00%d\x00%d\x00%d\x00%d\x01" per window into a
// SHA-256 stream. The \x00 separator and \x01 row terminator make
// boundaries unambiguous (a title containing a digit cannot blur into the
// next window's app name).
//
// Deliberately NOT hashed: window IDs (unstable across remap), z-order
// (changes on every focus change), focused-flag, display index. See
// 07-RESEARCH.md "Pattern 2".
func HashWindows(windows []Window) string {
	// Defensive copy — caller must not see their slice reordered.
	sorted := make([]Window, len(windows))
	copy(sorted, windows)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.AppName != b.AppName {
			return a.AppName < b.AppName
		}
		if a.WindowTitle != b.WindowTitle {
			return a.WindowTitle < b.WindowTitle
		}
		if a.X != b.X {
			return a.X < b.X
		}
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.W != b.W {
			return a.W < b.W
		}
		return a.H < b.H
	})

	h := sha256.New()
	for _, w := range sorted {
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d\x00%d\x00%d\x01",
			w.AppName, w.WindowTitle, w.X, w.Y, w.W, w.H)
	}
	return hex.EncodeToString(h.Sum(nil))
}
