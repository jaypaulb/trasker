// cmd/trasker-client/cli.go
//
// Subcommand dispatch for `trasker-client`. The bare invocation
// (no args) launches the daemon — backwards-compatible with the
// way the binary has shipped since Phase 5. The new subcommands
// (`status`, `open`, `quit`, `install-autostart`) operate on a
// daemon that is already running, by talking to its local web UI
// over loopback HTTP.
//
// Design choices:
//
//   - No flag.NewFlagSet, no cobra. Subcommands are positional and
//     all have <2 args; a single switch is plenty.
//   - The CLI commands print to stdout/stderr directly and call
//     os.Exit. They do not pull in the daemon's heavy init path
//     (database open, sync, tracker, ...) because those costs
//     would slow `trasker-client status` from milliseconds to
//     seconds and serialize against a running daemon's SQLite.
//   - Output is plain key=value style (no markdown tables) per
//     the project-wide formatting rule.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	clientruntime "github.com/jaypaulb/trasker/internal/client/runtime"
)

// dispatch parses argv[1] and routes to the appropriate subcommand.
// It returns true if a subcommand handled the call (caller exits via
// the subcommand's os.Exit), false if argv has no recognized command
// (caller falls through to runDaemon for backwards-compatibility).
//
// `version` and `--version` are recognized as well so the binary can
// answer `trasker-client --version` without spinning up the daemon.
func dispatch(argv []string, ver string) bool {
	if len(argv) < 2 {
		return false // bare invocation → daemon
	}
	switch argv[1] {
	case "status":
		os.Exit(runStatus())
	case "open":
		os.Exit(runOpen())
	case "quit":
		os.Exit(runQuit())
	case "install-autostart":
		os.Exit(runInstallAutostart())
	case "version", "--version", "-v":
		fmt.Println(ver)
		os.Exit(0)
	case "help", "--help", "-h":
		fmt.Print(usage)
		os.Exit(0)
	}
	// Unknown subcommand — print usage and exit non-zero.
	fmt.Fprintf(os.Stderr, "trasker-client: unknown command %q\n\n%s", argv[1], usage)
	os.Exit(2)
	return true
}

const usage = `Usage:
  trasker-client                   Launch the daemon (default)
  trasker-client status            Show daemon health (exit 1 if not running)
  trasker-client open              Open the dashboard in the default browser
  trasker-client quit              Ask the running daemon to exit cleanly
  trasker-client install-autostart Write ~/.config/autostart/trasker-client.desktop
  trasker-client version           Print version
  trasker-client help              Print this help
`

// runStatus implements `trasker-client status`. Exit codes:
//
//	0 — daemon running, status printed
//	1 — daemon not running (or stale state)
//	2 — daemon running but /api/status unreachable (rare; prints what we have)
func runStatus() int {
	st, err := clientruntime.ReadState()
	if err != nil {
		if errors.Is(err, clientruntime.ErrNotRunning) {
			fmt.Fprintln(os.Stdout, "trasker daemon: not running")
			return 1
		}
		fmt.Fprintf(os.Stderr, "trasker-client status: %v\n", err)
		return 1
	}

	// Try to fetch live metrics from the daemon.
	live, fetchErr := fetchStatus(st.URL)
	if fetchErr != nil {
		// Pidfile alive but HTTP unreachable. Print what we know
		// and warn — could be a port mismatch or the webui hasn't
		// finished starting.
		fmt.Printf("status:        running (metrics unreachable: %v)\n", fetchErr)
		fmt.Printf("pid:           %d\n", st.PID)
		fmt.Printf("url:           %s\n", st.URL)
		if !st.StartedAt.IsZero() {
			fmt.Printf("started_at:    %s\n", st.StartedAt.UTC().Format(time.RFC3339))
		}
		return 2
	}

	fmt.Printf("status:        running\n")
	fmt.Printf("pid:           %d\n", live.PID)
	fmt.Printf("port:          %d\n", live.Port)
	fmt.Printf("url:           %s\n", st.URL)
	if live.StartedAt != "" {
		fmt.Printf("started_at:    %s\n", live.StartedAt)
	}
	fmt.Printf("uptime:        %s\n", formatUptime(live.UptimeSeconds))
	fmt.Printf("presence:      %s\n", orUnknown(live.PresenceState))
	fmt.Printf("screen_lock:   %s\n", orUnknown(live.ScreenLockState))
	fmt.Printf("last_layout:   %s\n", orNever(live.LastLayoutSnap))
	fmt.Printf("last_sync:     %s\n", orNever(live.LastServerSync))
	return 0
}

// runOpen implements `trasker-client open`. Reads the URL from the
// state file and shells to the platform's URL handler. Exit 1 if
// the daemon isn't running (no URL to open).
func runOpen() int {
	st, err := clientruntime.ReadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, "trasker-client open: daemon not running")
		return 1
	}
	if err := openURL(st.URL); err != nil {
		fmt.Fprintf(os.Stderr, "trasker-client open: %v\n", err)
		return 1
	}
	return 0
}

// runQuit implements `trasker-client quit`. POSTs to /api/quit on
// the daemon's web UI. Exit 1 if the daemon isn't running, 0 if
// the request was accepted, 2 if the request reached the daemon
// but the daemon refused.
func runQuit() int {
	st, err := clientruntime.ReadState()
	if err != nil {
		fmt.Fprintln(os.Stdout, "trasker daemon: not running")
		return 1
	}

	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, st.URL+"/api/quit", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trasker-client quit: build request: %v\n", err)
		return 1
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trasker-client quit: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(os.Stderr, "trasker-client quit: HTTP %d\n", resp.StatusCode)
		return 2
	}
	fmt.Println("trasker-client quit: shutdown requested")
	return 0
}

// runInstallAutostart implements `trasker-client install-autostart`.
// Writes ~/.config/autostart/trasker-client.desktop pointing at the
// current binary. Idempotent — re-running overwrites.
//
// Note: the existing internal/client/setup/autostart_linux.go writes
// `trasker.desktop` (used by the tray's "Start with OS" toggle).
// This subcommand intentionally writes a *different* filename
// (`trasker-client.desktop`) so a CLI install does not silently
// disable the tray-managed entry on next startup, and vice-versa.
// The Phase-10 success criteria explicitly name `trasker-client.desktop`.
func runInstallAutostart() int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "trasker-client install-autostart: resolve binary path: %v\n", err)
		return 1
	}
	// Resolve symlinks so the desktop file points at the real binary.
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}

	path, err := writeAutostartDesktop(exe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trasker-client install-autostart: %v\n", err)
		return 1
	}
	fmt.Printf("trasker-client install-autostart: wrote %s\n", path)
	return 0
}

// writeAutostartDesktop writes the trasker-client.desktop autostart
// entry under XDG_CONFIG_HOME/autostart (default ~/.config/autostart).
// Returns the absolute path written. Overwrites any existing file —
// idempotent re-run is the explicit success criterion.
//
// On non-Linux platforms this still works (we just write the file
// at $HOME/.config/autostart/trasker-client.desktop), but it's a
// no-op as far as the OS is concerned. The mac/windows native
// autostart paths live in internal/client/setup; the CLI subcommand
// keeps the spec-named filename instead of dispatching, because the
// Phase-10 success criteria mention the exact path.
func writeAutostartDesktop(execPath string) (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}
	dir := filepath.Join(configDir, "autostart")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}

	path := filepath.Join(dir, "trasker-client.desktop")
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Trasker Client
Comment=Trasker time-tracking daemon
Exec=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`, execPath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// statusResponse mirrors the JSON shape returned by /api/status.
// Kept private to this file — the daemon owns the canonical type.
type statusResponse struct {
	PID             int    `json:"pid"`
	Port            int    `json:"port"`
	StartedAt       string `json:"started_at,omitempty"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
	PresenceState   string `json:"presence_state,omitempty"`
	ScreenLockState string `json:"screen_lock_state,omitempty"`
	LastLayoutSnap  string `json:"last_layout_snapshot,omitempty"`
	LastServerSync  string `json:"last_server_sync,omitempty"`
}

// fetchStatus GETs /api/status and decodes it. 3-second timeout so
// `trasker-client status` doesn't hang forever if the webui is wedged.
func fetchStatus(baseURL string) (*statusResponse, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/api/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var s statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &s, nil
}

// openURL shells to the platform's default URL handler. Stays in
// stdlib (os/exec) — no new deps. Mirrors setup.OpenBrowser but is
// duplicated here intentionally so cmd/trasker-client/cli.go can
// be exercised without dragging in the entire setup package's
// platform-specific autostart code.
func openURL(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "linux":
		cmd, args = "xdg-open", []string{url}
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return exec.Command(cmd, args...).Start()
}

// formatUptime renders a duration in seconds as e.g. "1h23m45s" or
// "47s". Avoids time.Duration.String() because it produces
// less-readable strings like "1h23m45.123456789s".
func formatUptime(secs int64) string {
	if secs < 0 {
		return "0s"
	}
	d := time.Duration(secs) * time.Second
	return d.Truncate(time.Second).String()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func orNever(s string) string {
	if s == "" {
		return "never"
	}
	return s
}
