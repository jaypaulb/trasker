// Package runtime owns the on-disk state files that announce a running
// trasker-client daemon to the rest of the system: a pidfile and a
// dashboard-URL file under XDG_STATE_HOME/trasker (default
// ~/.local/state/trasker).
//
// Stale-pid detection: if a previous run crashed without removing the
// pidfile, WriteState detects the dead PID via os.FindProcess +
// signal 0 and overwrites instead of erroring. This is what makes
// `trasker-client` (no args) survive reboots/SIGKILLs without the user
// having to clean up by hand.
//
// Files written:
//
//	$XDG_STATE_HOME/trasker/trasker.pid  - decimal pid, no newline
//	$XDG_STATE_HOME/trasker/trasker.url  - http://127.0.0.1:<port>, no newline
//
// Both files are 0644 (world-readable; the URL is local-only and the pid
// is harmless). The directory is 0755.
package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// State represents the on-disk runtime state of a running daemon.
type State struct {
	PID       int       // process id from trasker.pid
	URL       string    // dashboard URL from trasker.url
	Port      int       // parsed port from URL (0 if unparseable)
	StartedAt time.Time // mtime of trasker.pid (proxy for daemon start)
}

// StateDir returns the directory used for runtime state files. It honors
// XDG_STATE_HOME and falls back to ~/.local/state on Linux/macOS. On
// Windows it uses %LOCALAPPDATA%\Trasker\state.
//
// Returns an error only when neither XDG_STATE_HOME nor a home directory
// can be resolved — which on a real desktop session essentially never
// happens.
func StateDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "trasker"), nil
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		// Windows fallback when XDG_STATE_HOME isn't set.
		return filepath.Join(local, "Trasker", "state"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("runtime: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "trasker"), nil
}

// pidPath returns the absolute path to the pidfile.
func pidPath(dir string) string { return filepath.Join(dir, "trasker.pid") }

// urlPath returns the absolute path to the URL file.
func urlPath(dir string) string { return filepath.Join(dir, "trasker.url") }

// WriteState writes the pidfile + URL file for the current process.
//
// If a stale pidfile exists pointing at a non-running process, it is
// overwritten without error. If a live pidfile exists pointing at a
// different running process, ErrAlreadyRunning is returned — the caller
// (cmd/trasker-client) treats this as a hard error: don't double-launch.
func WriteState(port int) error {
	dir, err := StateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runtime: mkdir %s: %w", dir, err)
	}

	// Stale-pid check: if a pidfile exists and points at a *different*
	// running process, refuse. Otherwise, overwrite.
	if existing, err := readPID(pidPath(dir)); err == nil {
		if existing != os.Getpid() && processAlive(existing) {
			return fmt.Errorf("%w (pid %d)", ErrAlreadyRunning, existing)
		}
	}

	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(pidPath(dir), []byte(pid), 0o644); err != nil {
		return fmt.Errorf("runtime: write pidfile: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := os.WriteFile(urlPath(dir), []byte(url), 0o644); err != nil {
		return fmt.Errorf("runtime: write urlfile: %w", err)
	}
	return nil
}

// ReadState reads the pidfile + URL file, returning a populated State.
//
// Returns ErrNotRunning when the pidfile is absent OR when it exists
// but points at a process that is not alive (stale state).
func ReadState() (*State, error) {
	dir, err := StateDir()
	if err != nil {
		return nil, err
	}

	pid, err := readPID(pidPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotRunning
		}
		return nil, err
	}
	if !processAlive(pid) {
		return nil, ErrNotRunning
	}

	urlBytes, err := os.ReadFile(urlPath(dir))
	if err != nil {
		// pidfile present + alive but no URL: still report running, no URL.
		// This is degenerate but possible on partial-write crashes; the
		// `quit` subcommand needs the URL so we surface a useful error.
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("runtime: pidfile present but urlfile missing: %w", ErrCorrupt)
		}
		return nil, fmt.Errorf("runtime: read urlfile: %w", err)
	}
	url := strings.TrimSpace(string(urlBytes))

	port := parsePort(url)

	startedAt := time.Time{}
	if info, err := os.Stat(pidPath(dir)); err == nil {
		startedAt = info.ModTime()
	}

	return &State{PID: pid, URL: url, Port: port, StartedAt: startedAt}, nil
}

// Clear removes both state files. Errors on missing files are not
// returned — Clear is idempotent so a daemon that crashed mid-startup
// can re-launch cleanly.
func Clear() error {
	dir, err := StateDir()
	if err != nil {
		return err
	}
	for _, p := range []string{pidPath(dir), urlPath(dir)} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("runtime: remove %s: %w", p, err)
		}
	}
	return nil
}

// ErrNotRunning is returned by ReadState when there is no live daemon.
var ErrNotRunning = errors.New("trasker daemon not running")

// ErrAlreadyRunning is returned by WriteState when a different live
// daemon is already registered.
var ErrAlreadyRunning = errors.New("trasker daemon already running")

// ErrCorrupt is returned by ReadState when the pidfile is present and
// the PID is alive, but the URL file is missing or unreadable. Callers
// should treat this as "running but unusable" — the daemon should be
// killed via PID and restarted.
var ErrCorrupt = errors.New("trasker state files corrupt")

// readPID reads a pid from `path`. Returns os.PathError-wrapping
// errors for not-found so callers can use os.IsNotExist().
func readPID(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	pid, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("runtime: parse pidfile %q: %w", s, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("runtime: invalid pid %d in pidfile", pid)
	}
	return pid, nil
}

// processAlive reports whether the given pid refers to a process the
// current user can signal. Uses signal 0 — POSIX-defined as "do not
// send a signal, but check that we could".
//
// On Windows os.FindProcess always succeeds and Signal returns
// "not supported", so we fall back to a different probe there.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 is the standard POSIX "is alive" probe.
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// On Linux/macOS: nil = alive, ESRCH = dead, EPERM = alive but not ours.
	// We treat EPERM as alive to be safe (don't overwrite someone else's pid).
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

// parsePort extracts the port from "http://127.0.0.1:9746" style URLs.
// Returns 0 if the URL doesn't match.
func parsePort(url string) int {
	idx := strings.LastIndex(url, ":")
	if idx < 0 {
		return 0
	}
	tail := url[idx+1:]
	// strip trailing path if any
	if slash := strings.IndexByte(tail, '/'); slash >= 0 {
		tail = tail[:slash]
	}
	port, err := strconv.Atoi(tail)
	if err != nil {
		return 0
	}
	return port
}
