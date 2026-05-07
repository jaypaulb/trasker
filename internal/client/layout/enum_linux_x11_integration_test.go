//go:build linux && cgo && x11_integration

package layout

import (
	"os"
	"testing"
)

// TestX11Enumerate_RealDisplay opens a real X11 display and asserts at
// least one window is enumerated with non-negative geometry. NOT run on
// CI; invoked manually via:
//
//	DISPLAY=:0 go test -tags x11_integration ./internal/client/layout/...
func TestX11Enumerate_RealDisplay(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY not set; X11 integration test cannot run")
	}

	enum, err := NewX11Enumerator()
	if err != nil {
		t.Fatalf("NewX11Enumerator() error: %v", err)
	}
	defer enum.Close()

	windows, err := enum.Enumerate()
	if err != nil {
		t.Fatalf("Enumerate() error: %v", err)
	}
	if len(windows) == 0 {
		t.Fatal("expected at least 1 window, got 0")
	}
	for i, w := range windows {
		if w.W < 0 || w.H < 0 {
			t.Errorf("window %d has negative size: %+v", i, w)
		}
	}
}
