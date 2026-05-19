package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// withStateDir points XDG_STATE_HOME at a temporary directory for the
// duration of t and restores the previous value on cleanup. This is the
// only way to redirect StateDir() since runtime/ has no injectable
// dependencies (deliberate — keep the package shape tiny).
func withStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	// Also clear LOCALAPPDATA so the Windows fallback doesn't kick in
	// during cross-platform testing.
	t.Setenv("LOCALAPPDATA", "")
	return filepath.Join(dir, "trasker")
}

func TestStateDir_HonorsXDGStateHome(t *testing.T) {
	want := withStateDir(t)
	got, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if got != want {
		t.Fatalf("StateDir = %q, want %q", got, want)
	}
}

func TestWriteState_CreatesBothFiles(t *testing.T) {
	dir := withStateDir(t)

	if err := WriteState(9746); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	pidBytes, err := os.ReadFile(filepath.Join(dir, "trasker.pid"))
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("pid in file = %d, want %d", pid, os.Getpid())
	}

	urlBytes, err := os.ReadFile(filepath.Join(dir, "trasker.url"))
	if err != nil {
		t.Fatalf("read urlfile: %v", err)
	}
	want := "http://127.0.0.1:9746"
	if got := strings.TrimSpace(string(urlBytes)); got != want {
		t.Fatalf("url in file = %q, want %q", got, want)
	}
}

func TestReadState_RoundTrip(t *testing.T) {
	withStateDir(t)
	if err := WriteState(9999); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	st, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if st.PID != os.Getpid() {
		t.Fatalf("PID = %d, want %d", st.PID, os.Getpid())
	}
	if st.Port != 9999 {
		t.Fatalf("Port = %d, want 9999", st.Port)
	}
	if st.URL != "http://127.0.0.1:9999" {
		t.Fatalf("URL = %q, want http://127.0.0.1:9999", st.URL)
	}
	if st.StartedAt.IsZero() {
		t.Fatalf("StartedAt should not be zero")
	}
}

func TestReadState_NotRunning_NoFile(t *testing.T) {
	withStateDir(t)
	_, err := ReadState()
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("ReadState = %v, want ErrNotRunning", err)
	}
}

func TestReadState_StalePID_ReturnsNotRunning(t *testing.T) {
	dir := withStateDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pid 0 is invalid (will fail processAlive) — readPID rejects 0
	// directly, so use a negative number written manually.
	// Better: a definitely-dead high pid. Use pid 1 negated... actually
	// readPID rejects <= 0. So write a high pid that's almost certainly
	// dead. Pick a value above any plausible kernel.pid_max.
	// Linux defaults max to 4194304; using 4194303 is risky. Instead,
	// use an arbitrarily-allocated child we kill immediately.
	cmd := exec_RunDeadChild(t)

	if err := os.WriteFile(filepath.Join(dir, "trasker.pid"),
		[]byte(strconv.Itoa(cmd)), 0o644); err != nil {
		t.Fatalf("write fake pidfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trasker.url"),
		[]byte("http://127.0.0.1:1234"), 0o644); err != nil {
		t.Fatalf("write fake urlfile: %v", err)
	}

	_, err := ReadState()
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("ReadState with stale pid = %v, want ErrNotRunning", err)
	}
}

func TestWriteState_OverwritesStalePID(t *testing.T) {
	dir := withStateDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Plant a stale pidfile pointing at a dead pid.
	deadPID := exec_RunDeadChild(t)
	if err := os.WriteFile(filepath.Join(dir, "trasker.pid"),
		[]byte(strconv.Itoa(deadPID)), 0o644); err != nil {
		t.Fatalf("plant stale pid: %v", err)
	}

	// WriteState should overwrite, not error.
	if err := WriteState(8080); err != nil {
		t.Fatalf("WriteState over stale pid: %v", err)
	}

	st, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState after overwrite: %v", err)
	}
	if st.PID != os.Getpid() {
		t.Fatalf("PID after overwrite = %d, want %d", st.PID, os.Getpid())
	}
}

func TestWriteState_OverwritesReusedPID(t *testing.T) {
	dir := withStateDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Plant a pidfile with a live PID that is NOT trasker-client.
	// os.Getppid() is always alive and always a different binary (the test runner).
	reusedPID := os.Getppid()
	if err := os.WriteFile(filepath.Join(dir, "trasker.pid"),
		[]byte(strconv.Itoa(reusedPID)), 0o644); err != nil {
		t.Fatalf("plant reused pid: %v", err)
	}

	// WriteState must overwrite (not error) — the live PID is not ours.
	if err := WriteState(8081); err != nil {
		t.Fatalf("WriteState over reused pid: %v (pid %d)", err, reusedPID)
	}

	st, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState after overwrite: %v", err)
	}
	if st.PID != os.Getpid() {
		t.Fatalf("PID after overwrite = %d, want %d", st.PID, os.Getpid())
	}
}

func TestClear_Idempotent(t *testing.T) {
	withStateDir(t)
	// Clear with no files — should not error.
	if err := Clear(); err != nil {
		t.Fatalf("Clear (empty): %v", err)
	}
	// Now write and clear.
	if err := WriteState(7000); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if err := Clear(); err != nil {
		t.Fatalf("Clear (populated): %v", err)
	}
	// Both files should be gone.
	dir, _ := StateDir()
	if _, err := os.Stat(filepath.Join(dir, "trasker.pid")); !os.IsNotExist(err) {
		t.Fatalf("pidfile not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "trasker.url")); !os.IsNotExist(err) {
		t.Fatalf("urlfile not removed: %v", err)
	}
}

func TestParsePort(t *testing.T) {
	cases := []struct {
		url  string
		want int
	}{
		{"http://127.0.0.1:9746", 9746},
		{"http://127.0.0.1:9746/dashboard", 9746},
		{"http://localhost:8080", 8080},
		{"not a url", 0},
		{"http://no-port", 0},
	}
	for _, c := range cases {
		if got := parsePort(c.url); got != c.want {
			t.Errorf("parsePort(%q) = %d, want %d", c.url, got, c.want)
		}
	}
}

// exec_RunDeadChild spawns a child process that immediately exits and
// returns its (now-dead) pid. This is more reliable than picking a
// "probably dead" pid because the kernel guarantees the pid won't be
// reused for a long time on most systems and we wait for the child to
// reap. Underscore in the name so it's clearly a test helper.
func exec_RunDeadChild(t *testing.T) int {
	t.Helper()
	// Use os.StartProcess with /bin/true — a no-op that exits 0.
	// This avoids importing os/exec for one helper.
	bin, err := lookupTrueBinary()
	if err != nil {
		t.Skipf("can't find a no-op binary for stale-pid test: %v", err)
	}
	proc, err := os.StartProcess(bin, []string{bin}, &os.ProcAttr{})
	if err != nil {
		t.Fatalf("start dead child: %v", err)
	}
	state, err := proc.Wait()
	if err != nil {
		t.Fatalf("wait dead child: %v", err)
	}
	if !state.Exited() {
		t.Fatalf("dead child did not exit cleanly: %v", state)
	}
	return proc.Pid
}

// lookupTrueBinary finds an executable that exits 0 immediately.
// /bin/true on Linux/macOS, no Windows path (test will skip).
func lookupTrueBinary() (string, error) {
	for _, p := range []string{"/bin/true", "/usr/bin/true"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no /bin/true")
}
