package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// withStateDir redirects XDG_STATE_HOME to a temp dir so writeAutostartDesktop
// and the runtime package use isolated paths.
func withStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("LOCALAPPDATA", "")
	return dir
}

func TestDispatch_BareInvocation(t *testing.T) {
	if dispatch([]string{"trasker-client"}, "test") {
		t.Fatal("dispatch with no args should return false (fall through to daemon)")
	}
}

func TestRunStatus_NotRunning(t *testing.T) {
	withStateDir(t)
	code := runStatus()
	if code != 1 {
		t.Fatalf("runStatus when not running = %d, want 1", code)
	}
}

func TestRunQuit_NotRunning(t *testing.T) {
	withStateDir(t)
	code := runQuit()
	if code != 1 {
		t.Fatalf("runQuit when not running = %d, want 1", code)
	}
}

func TestRunQuit_HitsRunningDaemon(t *testing.T) {
	stateDir := withStateDir(t)

	// Start a tiny test server that records the /api/quit hit.
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/quit" && r.Method == http.MethodPost {
			hits++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"shutting_down"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Plant a state file pointing at this test server. We use the
	// current process's pid so processAlive returns true.
	dir := filepath.Join(stateDir, "trasker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trasker.pid"),
		[]byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trasker.url"),
		[]byte(srv.URL), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runQuit()
	if code != 0 {
		t.Fatalf("runQuit = %d, want 0", code)
	}
	if hits != 1 {
		t.Fatalf("expected 1 /api/quit hit, got %d", hits)
	}
}

func TestRunStatus_FetchesFromDaemon(t *testing.T) {
	stateDir := withStateDir(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(statusResponse{
				PID:             os.Getpid(),
				Port:            8765,
				StartedAt:       "2026-05-08T08:00:00Z",
				UptimeSeconds:   3600,
				PresenceState:   "TRACKING",
				ScreenLockState: "UNLOCKED",
				LastLayoutSnap:  "2026-05-08T08:55:00Z",
				LastServerSync:  "2026-05-08T08:50:00Z",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := filepath.Join(stateDir, "trasker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "trasker.pid"),
		[]byte(strconv.Itoa(os.Getpid())), 0o644)
	os.WriteFile(filepath.Join(dir, "trasker.url"),
		[]byte(srv.URL), 0o644)

	// Capture stdout — runStatus prints there.
	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	code := runStatus()
	w.Close()
	out, _ := readAll(r)

	if code != 0 {
		t.Fatalf("runStatus = %d, want 0\noutput: %s", code, out)
	}
	for _, want := range []string{
		"status:        running",
		"pid:",
		"port:          8765",
		"presence:      TRACKING",
		"screen_lock:   UNLOCKED",
		"last_layout:   2026-05-08T08:55:00Z",
		"last_sync:     2026-05-08T08:50:00Z",
		"uptime:        1h0m0s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestWriteAutostartDesktop_Idempotent(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	exe := "/usr/local/bin/trasker-client"
	path1, err := writeAutostartDesktop(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path1, "/autostart/trasker-client.desktop") {
		t.Errorf("path = %q, expected to end with /autostart/trasker-client.desktop", path1)
	}

	// Second call must not error and must overwrite cleanly.
	path2, err := writeAutostartDesktop(exe)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if path1 != path2 {
		t.Errorf("path mismatch: %q vs %q", path1, path2)
	}

	content, err := os.ReadFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Exec="+exe) {
		t.Errorf("desktop file missing Exec=%s\ncontent:\n%s", exe, content)
	}
	if !strings.Contains(string(content), "Type=Application") {
		t.Errorf("desktop file missing Type=Application\ncontent:\n%s", content)
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		secs int64
		want string
	}{
		{0, "0s"},
		{59, "59s"},
		{60, "1m0s"},
		{3661, "1h1m1s"},
		{-1, "0s"},
	}
	for _, c := range cases {
		if got := formatUptime(c.secs); got != c.want {
			t.Errorf("formatUptime(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestOrUnknownOrNever(t *testing.T) {
	if got := orUnknown(""); got != "unknown" {
		t.Errorf("orUnknown(\"\") = %q", got)
	}
	if got := orUnknown("foo"); got != "foo" {
		t.Errorf("orUnknown(\"foo\") = %q", got)
	}
	if got := orNever(""); got != "never" {
		t.Errorf("orNever(\"\") = %q", got)
	}
	if got := orNever("2026-05-08"); got != "2026-05-08" {
		t.Errorf("orNever(date) = %q", got)
	}
}

// readAll reads from an *os.File until EOF without pulling in io/ioutil.
func readAll(f *os.File) (string, error) {
	var buf [4096]byte
	var sb strings.Builder
	for {
		n, err := f.Read(buf[:])
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return sb.String(), nil
}
