# Phase 7: Layout Snapshots - Pattern Map

- **Mapped:** 2026-05-07
- **Files analyzed:** 23 (Wave 0 list from VALIDATION.md)
- **Analogs found:** 22 / 23 (one frontend route has only a partial match — see No Analog Found)
- **Output policy:** Bullet/heading format only. No markdown tables (per Jaypaul global rule).

---

## File Classification

Each file is listed with role, data flow, closest analog, and match quality (exact = same role + same data flow; role-match = same role, different data flow; partial = different role but useful patterns).

### Client Go files

- `internal/client/layout/layout.go`
  - Role: model (struct + pure hash function)
  - Data flow: transform
  - Closest analog: `internal/client/tracker/tracker.go` (interface + value type definitions)
  - Match quality: role-match

- `internal/client/layout/layout_test.go`
  - Role: test (pure unit, no DB, no I/O)
  - Data flow: transform
  - Closest analog: `internal/client/sync/queue_test.go` `TestQueue_BackoffSchedule` (pure-function table test in same package style)
  - Match quality: role-match

- `internal/client/layout/enum_linux_x11.go`
  - Role: utility (cgo-backed enumerator implementing `Enumerator` interface)
  - Data flow: transform (X11 state -> []Window)
  - Closest analog: `internal/client/tracker/tracker_linux_x11.go`
  - Match quality: exact

- `internal/client/layout/enum_linux_nocgo.go`
  - Role: utility (build-tag stub)
  - Data flow: transform (returns ErrUnsupported)
  - Closest analog: `internal/client/tracker/tracker_linux_nocgo.go`
  - Match quality: exact

- `internal/client/layout/enum_darwin.go` and `enum_windows.go`
  - Role: utility (build-tag stubs)
  - Data flow: transform (returns ErrUnsupported)
  - Closest analog: `internal/client/tracker/tracker_darwin_nocgo.go` (returns sentinel error in `NewPlatformTracker`)
  - Match quality: role-match

- `internal/client/layout/capturer.go`
  - Role: service (organism: ticker + lock-gating + change-detect + writes)
  - Data flow: event-driven (ticker + screenlock channel)
  - Closest analog: `internal/client/tracker/tracker_linux_x11.go` `poll()` (1Hz ticker + change-detect on `lastApp/lastTitle`)
  - Match quality: role-match (60s vs 1s; uses screenlock channel that focus tracker does not subscribe to)

- `internal/client/layout/capturer_test.go`
  - Role: test (capturer unit tests with fake Enumerator + mock screenlock)
  - Data flow: event-driven
  - Closest analog: `internal/client/tracker/tracker_linux_test.go` (small focused unit tests; uses fake)
  - Match quality: partial (testing pattern is similar; data flow differs)

- `internal/client/layout/store.go`
  - Role: store (SQLite accessors: Insert, ListPending, MarkSynced, Prune, ListInRange)
  - Data flow: CRUD
  - Closest analog: `internal/client/store/focus_events.go`
  - Match quality: exact

- `internal/client/layout/store_test.go`
  - Role: test (sqlite roundtrip + prune coverage)
  - Data flow: CRUD
  - Closest analog: `internal/client/store/focus_events_test.go`
  - Match quality: exact

- `internal/client/layout/sync.go`
  - Role: service (separate goroutine: ticker + retry/backoff + HTTP POST)
  - Data flow: event-driven (ticker) + request-response (POST)
  - Closest analog: `internal/client/sync/queue.go` (ticker + processPending + retry schedule)
  - Match quality: exact (mirror `Queue` deliberately; do NOT refactor it — Chesterton, Three Examples)

- `internal/client/layout/sync_test.go`
  - Role: test (httptest.NewServer based)
  - Data flow: request-response
  - Closest analog: `internal/client/sync/client_test.go` (httptest pattern, error sentinels) and `internal/client/sync/queue_test.go` (DB+httptest hybrid)
  - Match quality: exact

### Server Go files

- `internal/server/store/layout_snapshots.go`
  - Role: store (pgx accessors: Insert, GetAt, ListTimestamps, Downsample, Prune)
  - Data flow: CRUD
  - Closest analog: `internal/server/store/devices.go` (pgx pool, RETURNING, ON CONFLICT)
  - Match quality: exact

- `internal/server/store/layout_snapshots_test.go`
  - Role: test (testcontainers-go postgres, pgx pool, runs migrations from `migrations/`)
  - Data flow: CRUD
  - Closest analog: `internal/server/store/devices_test.go` + `testhelper_test.go` (already discovers and runs `migrations/*.up.sql`)
  - Match quality: exact

- `internal/server/api/layout_handlers.go`
  - Role: controller (Chi handlers: POST ingest API-key auth, GET timeline + GET ?t= JWT auth)
  - Data flow: request-response
  - Closest analog: `internal/server/api/timesheet_handlers.go` (POST under API key) + `internal/server/api/device_handlers.go` (GET list under JWT)
  - Match quality: exact

- `internal/server/api/layout_handlers_test.go`
  - Role: test (httptest + bcrypted API key + bearer header)
  - Data flow: request-response
  - Closest analog: `internal/server/api/device_handlers_test.go` + `testhelper_test.go`
  - Match quality: exact

### SQL migrations / initdb

- `migrations/004_layout_snapshots.up.sql` and `.down.sql`
  - Role: migration (Postgres DDL)
  - Data flow: file-I/O (loaded by tests via testhelper, applied manually via psql in production)
  - Closest analog: `migrations/001_init.up.sql` and `migrations/001_init.down.sql`
  - Match quality: exact

- `deploy/initdb/001_schema.sql` (APPEND only)
  - Role: config (Postgres init-on-empty-volume schema)
  - Data flow: file-I/O
  - Closest analog: existing `deploy/initdb/001_schema.sql` (already includes 001+002+003 inlined)
  - Match quality: exact (modify the same file)

### Web / SvelteKit

- `web/server-ui/src/routes/(app)/layout/+page.ts`
  - Role: route (SvelteKit `load`)
  - Data flow: request-response
  - Closest analog: NONE in tree — `(app)/devices/+page.svelte` uses `onMount` rather than a `+page.ts` loader
  - Match quality: see "No Analog Found" section; pattern comes from RESEARCH.md + UI-SPEC

- `web/server-ui/src/routes/(app)/layout/+page.svelte`
  - Role: component (page composition)
  - Data flow: request-response
  - Closest analog: `web/server-ui/src/routes/(app)/devices/+page.svelte`
  - Match quality: exact (mirror loading/error/empty states verbatim)

- `web/server-ui/src/lib/components/LayoutSnapshotRow.svelte`
  - Role: component (atom: timestamp row + chevron + expand state)
  - Data flow: event-driven (click)
  - Closest analog: `web/server-ui/src/routes/(app)/devices/+page.svelte` table row block (lines 119-152) and the `<details>` pattern at lines 82-90
  - Match quality: partial (no existing component is exactly a click-to-expand row)

- `web/server-ui/src/lib/components/LayoutSnapshotWindowList.svelte`
  - Role: component (atom: list of {app, title, geometry})
  - Data flow: transform (props -> markup)
  - Closest analog: the `tbody` rows in `(app)/devices/+page.svelte` (presentation pattern)
  - Match quality: partial

- `web/server-ui/src/lib/types.ts` (additive)
  - Role: model (TS interface)
  - Data flow: transform
  - Closest analog: `Device` interface in `web/server-ui/src/lib/types.ts` (lines 13-22)
  - Match quality: exact

- `web/server-ui/src/lib/api.ts` (additive `layout` namespace)
  - Role: utility (HTTP client wrapper)
  - Data flow: request-response
  - Closest analog: `timesheets` namespace in `web/server-ui/src/lib/api.ts` (lines 143-159)
  - Match quality: exact

- `web/server-ui/src/lib/components/Sidebar.svelte` (single-line edit)
  - Role: config (nav item array)
  - Data flow: transform
  - Closest analog: existing `Devices` entry at line 14 of `Sidebar.svelte`
  - Match quality: exact (append between Devices and Team per UI-SPEC)

---

## Pattern Assignments

Each entry below cites file paths and line numbers from the existing trasker codebase. Excerpts are verbatim copies meant to be adapted, not summaries.

### `internal/client/layout/enum_linux_x11.go` (utility, cgo X11)

**Analog:** `internal/client/tracker/tracker_linux_x11.go`

**Build tag header (line 1):**

```go
//go:build linux && cgo
```

**Cgo block — open display + atom interning + property fetch fallback chain (lines 6-65):**

```go
/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <string.h>

// getActiveWindowInfo retrieves the app name (WM_CLASS) and window title (_NET_WM_NAME or WM_NAME)
// of the currently focused window. Returns 0 on success, -1 on failure.
static int getActiveWindowInfo(Display *dpy, char *app_name, int app_len, char *win_title, int title_len) {
    Window focused;
    int revert;
    XGetInputFocus(dpy, &focused, &revert);
    if (focused == None || focused == PointerRoot) {
        return -1;
    }

    // Get WM_CLASS for app name
    XClassHint class_hint;
    if (XGetClassHint(dpy, focused, &class_hint)) {
        if (class_hint.res_name) {
            strncpy(app_name, class_hint.res_name, app_len - 1);
            app_name[app_len - 1] = '\0';
            XFree(class_hint.res_name);
        }
        if (class_hint.res_class) {
            XFree(class_hint.res_class);
        }
    }

    // Try _NET_WM_NAME first (UTF-8), fall back to WM_NAME
    Atom net_wm_name = XInternAtom(dpy, "_NET_WM_NAME", True);
    Atom utf8_string = XInternAtom(dpy, "UTF8_STRING", True);
    ...
}
*/
import "C"
```

**Adaptation for Phase 7:** replace `XGetInputFocus` with `_NET_CLIENT_LIST_STACKING` via `XGetWindowProperty(... root ..., XA_WINDOW, ...)` and fall back to `XQueryTree(root, ...)`. Add a `getGeom()` C helper that calls `XGetWindowAttributes` AND `XTranslateCoordinates(dpy, w, root, 0, 0, &x, &y, &child)` (RESEARCH.md Pitfall 1: parent-relative coords are the #1 X11 footgun).

**Constructor pattern — open display, return error if nil (lines 89-98):**

```go
func NewX11Tracker() (*X11Tracker, error) {
    dpy := C.XOpenDisplay(nil)
    if dpy == nil {
        return nil, fmt.Errorf("cannot open X11 display (is DISPLAY set?)")
    }
    return &X11Tracker{
        display: dpy,
        events:  make(chan FocusChange, 32),
    }, nil
}
```

**Stack-allocated buffer + unsafe.Pointer pattern for cgo string return (lines 144-160):**

```go
func (t *X11Tracker) getActive() (string, string) {
    const bufSize = 512
    var appBuf [bufSize]C.char
    var titleBuf [bufSize]C.char

    ret := C.getActiveWindowInfo(
        t.display,
        (*C.char)(unsafe.Pointer(&appBuf[0])), C.int(bufSize),
        (*C.char)(unsafe.Pointer(&titleBuf[0])), C.int(bufSize),
    )
    if ret != 0 {
        return "", ""
    }

    return C.GoString((*C.char)(unsafe.Pointer(&appBuf[0]))),
        C.GoString((*C.char)(unsafe.Pointer(&titleBuf[0])))
}
```

---

### `internal/client/layout/enum_linux_nocgo.go` (utility, no-cgo stub)

**Analog:** `internal/client/tracker/tracker_linux_nocgo.go`

**Whole file pattern (lines 1-25):**

```go
//go:build linux && !cgo

// internal/client/tracker/tracker_linux_nocgo.go
// When CGO_ENABLED=0 (cross-compiling from Linux server), the X11 tracker
// is unavailable because it requires linking against libX11. ...
package tracker

import "fmt"

// NewPlatformTracker creates a focus tracker without CGo.
func NewPlatformTracker() (Tracker, error) {
    sessionType := DetectSessionType()
    switch sessionType {
    case "wayland":
        return NewWaylandTracker()
    case "x11":
        return nil, fmt.Errorf("X11 focus tracking requires a CGo-enabled build (this binary was cross-compiled without CGo)")
    ...
}
```

**Adaptation:** layout has no Wayland fallback in v1, so the stub is simpler — return `nil, ErrUnsupported` unconditionally. Define `ErrUnsupported = errors.New("layout enumeration not supported on this build/platform")` in `layout.go` (the no-build-tag file).

---

### `internal/client/layout/capturer.go` (service, ticker + change-detect + lock-gating)

**Analog (ticker + change-detect):** `internal/client/tracker/tracker_linux_x11.go` lines 121-142

```go
func (t *X11Tracker) poll(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            app, title := t.getActive()
            if app != t.lastApp || title != t.lastTitle {
                t.lastApp = app
                t.lastTitle = title
                t.events <- FocusChange{
                    AppName:     app,
                    WindowTitle: title,
                    Timestamp:   time.Now().UTC(),
                }
            }
        }
    }
}
```

**Adaptation for Phase 7:**
- Ticker is 60s, not 1s.
- Change-detect compares `windows_hash` (sha256 of canonical sorted-tuple form), not `(lastApp, lastTitle)`.
- On hash mismatch, INSERT a row into local sqlite (call `s.InsertSnapshot(...)`); do NOT push on a channel.
- Wrap the body in `if c.locked.Load() { return }` (D-08).
- On enum error, `slog.Warn(...)` and return — do NOT write a row (D-09).

**Analog (subscribe to screenlock events to maintain `locked` atomic.Bool):** `internal/client/presence/screenlock_linux.go` lines 79-101 + `presence/presence.go` lines 39-51

The screenlock listener is push-only — the capturer must subscribe and maintain its own bool:

```go
// Channel pattern from screenlock_linux.go (caller side: maintain atomic.Bool from these)
for scanner.Scan() {
    locked := strings.Contains(line, "true")
    state := Tracking
    if locked {
        state = Away
    }
    select {
    case l.events <- StateChange{
        State:     state,
        Timestamp: time.Now().UTC(),
    }:
    case <-ctx.Done():
        return
    }
}
```

**Capturer subscriber goroutine (Phase 7 sketch derived from this pattern):**

```go
// In Capturer.Start:
go func() {
    for ev := range screenLock.Events() {
        c.locked.Store(ev.State == presence.Away)
    }
}()
```

Use `presence.Away` (not `Paused`) — see RESEARCH.md A7. The `Paused` state belongs to deadman switch, not screenlock.

---

### `internal/client/layout/store.go` (store, sqlite CRUD)

**Analog:** `internal/client/store/focus_events.go`

**Imports + struct + insert pattern (lines 1-35):**

```go
package store

import (
    "database/sql"
    "fmt"
    "time"
)

// FocusEvent represents a row in the focus_events table.
type FocusEvent struct {
    ID          int64
    AppName     string
    WindowTitle string
    StartedAt   time.Time
    EndedAt     *time.Time
    DurationS   *int64
    IsIdle      bool
    CreatedAt   time.Time
}

// InsertFocusEvent inserts a new focus event and returns its ID.
func (s *Store) InsertFocusEvent(appName, windowTitle string, startedAt time.Time) (int64, error) {
    now := time.Now().UTC().Format(time.RFC3339)
    result, err := s.db.Exec(
        `INSERT INTO focus_events (app_name, window_title, started_at, created_at)
         VALUES (?, ?, ?, ?)`,
        appName, windowTitle, startedAt.Format(time.RFC3339), now,
    )
    if err != nil {
        return 0, fmt.Errorf("insert focus event: %w", err)
    }
    return result.LastInsertId()
}
```

**Range query pattern (lines 62-84) — use for "list snapshots since T" and "list pending sync":**

```go
func (s *Store) ListFocusEvents(from, to time.Time) ([]FocusEvent, error) {
    rows, err := s.db.Query(
        `SELECT id, app_name, window_title, started_at, ended_at, duration_s, is_idle, created_at
         FROM focus_events
         WHERE started_at >= ? AND started_at <= ?
         ORDER BY started_at ASC`,
        from.Format(time.RFC3339), to.Format(time.RFC3339),
    )
    if err != nil {
        return nil, fmt.Errorf("list focus events: %w", err)
    }
    defer rows.Close()

    var events []FocusEvent
    for rows.Next() {
        ev, err := scanFocusEventRow(rows)
        if err != nil {
            return nil, err
        }
        events = append(events, *ev)
    }
    return events, rows.Err()
}
```

**Scanner abstraction (lines 95-145) — copy whole pattern for `LayoutSnapshot` row scanning:**

```go
type scanner interface {
    Scan(dest ...any) error
}

func scanFocusEventFromScanner(sc scanner) (*FocusEvent, error) {
    var ev FocusEvent
    var startedAt, createdAt string
    var endedAt sql.NullString
    ...
    err := sc.Scan(...)
    ...
    ev.StartedAt, parseErr = time.Parse(time.RFC3339, startedAt)
    if parseErr != nil {
        return nil, fmt.Errorf("parse started_at %q: %w", startedAt, parseErr)
    }
    ...
}
```

**Schema-add pattern (sqlite migration is inline in store.go):** see `internal/client/store/store.go` lines 55-141. Append to the same `schema` literal:

```go
CREATE TABLE IF NOT EXISTS layout_snapshots (
    id           INTEGER PRIMARY KEY,
    captured_at  TEXT NOT NULL,
    windows      TEXT NOT NULL,        -- JSON array
    windows_hash TEXT NOT NULL,
    synced_at    TEXT,                 -- NULL = not yet synced
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_layout_snapshots_captured ON layout_snapshots(captured_at);
CREATE INDEX IF NOT EXISTS idx_layout_snapshots_pending  ON layout_snapshots(synced_at) WHERE synced_at IS NULL;
```

(Adapt schema to taste; the additive-string-to-`schema` move is the pattern, not the exact DDL.)

---

### `internal/client/layout/store_test.go` (test, sqlite roundtrip)

**Analog:** `internal/client/store/focus_events_test.go`

**Setup helper (lines 12-21) — copy verbatim, rename function:**

```go
func newTestStore(t *testing.T) *store.Store {
    t.Helper()
    dir := t.TempDir()
    s, err := store.New(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatalf("New() error: %v", err)
    }
    t.Cleanup(func() { s.Close() })
    return s
}
```

**Roundtrip test shape (lines 23-48) — copy structure, rename fields:**

```go
func TestInsertFocusEvent(t *testing.T) {
    s := newTestStore(t)

    now := time.Now().UTC()
    id, err := s.InsertFocusEvent("Firefox", "GitHub - trasker", now)
    if err != nil {
        t.Fatalf("InsertFocusEvent() error: %v", err)
    }
    ...
}
```

**Range-query test (lines 77-101) — use as `TestStore_Prune7Days` skeleton:**

```go
func TestListFocusEventsByTimeRange(t *testing.T) {
    s := newTestStore(t)
    base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
    for i, offset := range []time.Duration{0, 30 * time.Minute, 60 * time.Minute} {
        id, err := s.InsertFocusEvent("App", "Window", base.Add(offset))
        ...
    }
    events, err := s.ListFocusEvents(base, base.Add(45*time.Minute))
    ...
    if len(events) != 2 { ... }
}
```

---

### `internal/client/layout/sync.go` (service, separate sync goroutine)

**Analog:** `internal/client/sync/queue.go` (mirror, do NOT extend)

**Backoff schedule constant (lines 13-19) — copy verbatim:**

```go
var backoffSchedule = []time.Duration{
    1 * time.Minute,
    5 * time.Minute,
    15 * time.Minute,
    1 * time.Hour,
}
```

**Queue struct + Start/Stop pattern (lines 31-93):**

```go
type Queue struct {
    db       *sql.DB
    client   *Client
    deviceID string
    logger   *slog.Logger
    mu       gosync.Mutex
    running  bool
    stopCh   chan struct{}
}

func (q *Queue) Start(ctx context.Context) {
    q.mu.Lock()
    if q.running {
        q.mu.Unlock()
        return
    }
    q.running = true
    q.stopCh = make(chan struct{})
    q.mu.Unlock()

    go q.run(ctx)
}

func (q *Queue) run(ctx context.Context) {
    q.ProcessPending(ctx)
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-q.stopCh:
            return
        case <-ticker.C:
            q.ProcessPending(ctx)
        }
    }
}
```

**Backoff-due check + retry bumping (lines 112-159):**

```go
func (q *Queue) listDueSubmissions() ([]PendingSubmission, error) {
    rows, err := q.db.Query(
        `SELECT id, status, retry_count, last_retry
         FROM submissions
         WHERE status = 'pending'
         ORDER BY submitted_at ASC`,
    )
    ...
    if sub.LastRetry != nil {
        backoff := q.backoffFor(sub.RetryCount)
        if now.Before(sub.LastRetry.Add(backoff)) {
            continue // not due yet
        }
    }
    ...
}

func (q *Queue) backoffFor(retryCount int) time.Duration {
    if retryCount < len(backoffSchedule) {
        return backoffSchedule[retryCount]
    }
    return 1 * time.Hour
}
```

**Permanent vs transient error split (lines 183-205):**

```go
if err != nil {
    // Handle permanent errors (don't retry)
    if err == ErrKeyExpired || err == ErrKeyRevoked {
        q.logger.Error("permanent sync error", "submission_id", sub.ID, "error", err)
        return
    }
    // Transient error — bump retry
    q.logger.Warn("submission failed, will retry",
        "submission_id", sub.ID,
        "retry_count", sub.RetryCount+1,
        "error", err,
    )
    q.bumpRetry(sub.ID, sub.RetryCount)
    return
}
// Success
q.updateStatus(sub.ID, "confirmed", &resp.ID)
```

**Adaptation:** Phase 7 `Queue` operates on `layout_snapshots` rows where `synced_at IS NULL`. Replace `loadSubmissionEntries` (which JOINs `submission_events JOIN focus_events`) with a simple `SELECT id, captured_at, windows, windows_hash FROM layout_snapshots WHERE synced_at IS NULL`. On success, `UPDATE layout_snapshots SET synced_at = ? WHERE id = ?` instead of `UPDATE submissions SET status='confirmed'`. Reuse the existing `sync.Client` HTTP wrapper (next section) for the actual POST.

**HTTP client to reuse:** `internal/client/sync/client.go` lines 79-174. The Phase 7 sync goroutine should NOT create a new HTTP client; reuse `sync.Client` and add a `PostLayoutSnapshots(ctx, batch)` method that calls the same `c.post(ctx, "/api/v1/layout-snapshots", ...)` machinery. This shares `ErrKeyExpired` / `ErrKeyRevoked` sentinels.

---

### `internal/client/layout/sync_test.go` (test, httptest)

**Analog:** `internal/client/sync/client_test.go` + `internal/client/sync/queue_test.go`

**Single-handler httptest pattern (`client_test.go` lines 12-43):**

```go
func TestClient_RegisterDevice(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            t.Errorf("expected POST, got %s", r.Method)
        }
        if r.URL.Path != "/api/v1/devices" {
            t.Errorf("expected /api/v1/devices, got %s", r.URL.Path)
        }
        if r.Header.Get("Authorization") != "Bearer test-key" {
            t.Errorf("missing/wrong auth header")
        }
        ...
        w.WriteHeader(http.StatusCreated)
    }))
    defer server.Close()

    client := NewClient(server.URL, "test-key")
    err := client.RegisterDevice(context.Background(), DeviceRegistration{...})
    if err != nil {
        t.Fatalf("register: %v", err)
    }
}
```

**Permanent-error sentinel test (`client_test.go` lines 80-104) — use for `TestSync_KeyExpired`:**

```go
func TestClient_KeyExpired(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
    }))
    defer server.Close()

    client := NewClient(server.URL, "expired-key")
    err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
    if err != ErrKeyExpired {
        t.Errorf("expected ErrKeyExpired, got %v", err)
    }
}
```

**DB+httptest hybrid for retry/backoff (`queue_test.go` lines 18-54, 83-106):**

```go
func setupQueueDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    ...
    for _, stmt := range []string{
        `CREATE TABLE submissions (...)`,
        `INSERT INTO submissions ... VALUES (1, '...', 'pending')`,
    } {
        if _, err := db.Exec(stmt); err != nil {
            t.Fatalf("setup %q: %v", stmt[:40], err)
        }
    }
    return db
}

func TestQueue_ProcessPending_RetryOnError(t *testing.T) {
    db := setupQueueDB(t)
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer server.Close()
    ...
    var retryCount int
    db.QueryRow(`SELECT retry_count, last_retry FROM submissions WHERE id = 1`).Scan(&retryCount, &lastRetry)
    if retryCount != 1 {
        t.Errorf("expected retry_count 1, got %d", retryCount)
    }
}
```

**Adaptation:** seed `layout_snapshots` rows with `synced_at IS NULL`; assert post-run state via `SELECT synced_at FROM layout_snapshots WHERE id = 1` (success) or by reading retry counters if Phase 7 adds them (RESEARCH.md leaves this open — recommend a small `layout_snapshots_outbox` mirror table OR add `retry_count`/`last_retry` columns to `layout_snapshots` itself; planner picks).

---

### `internal/server/store/layout_snapshots.go` (store, pgx CRUD)

**Analog:** `internal/server/store/devices.go`

**Imports + struct + UPSERT pattern (lines 1-50):**

```go
package store

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
)

type Device struct {
    ID             uuid.UUID
    UserID         uuid.UUID
    APIKeyID       uuid.UUID
    ClientDeviceID string
    DeviceName     *string
    OS             string
    LastSeenAt     time.Time
    CreatedAt      time.Time
}

func (s *Store) UpsertDevice(ctx context.Context, p UpsertDeviceParams) (*Device, error) {
    var d Device
    err := s.pool.QueryRow(ctx,
        `INSERT INTO devices (user_id, api_key_id, client_device_id, os, device_name)
         VALUES ($1, $2, $3, $4, NULLIF($5, ''))
         ON CONFLICT (client_device_id, api_key_id) DO UPDATE SET last_seen_at = now()
         RETURNING id, user_id, api_key_id, client_device_id, device_name, os, last_seen_at, created_at`,
        p.UserID, p.APIKeyID, p.ClientDeviceID, p.OS, p.Hostname,
    ).Scan(&d.ID, &d.UserID, &d.APIKeyID, &d.ClientDeviceID, &d.DeviceName, &d.OS, &d.LastSeenAt, &d.CreatedAt)
    if err != nil {
        return nil, fmt.Errorf("upserting device: %w", err)
    }
    return &d, nil
}
```

**Adaptation:** Phase 7's INSERT mirror (server side):

```sql
INSERT INTO layout_snapshots (device_id, captured_at, windows, windows_hash, tier)
VALUES ($1, $2, $3, $4, 'raw')
ON CONFLICT (device_id, captured_at, windows_hash) DO NOTHING
RETURNING id, device_id, captured_at, windows, windows_hash, tier, created_at
```

The `ON CONFLICT ... DO NOTHING RETURNING` pattern returns zero rows on duplicate. RESEARCH.md Open Question #3 recommends treating both "insert returning id" and "no rows" as 201 from the client's perspective — handle the `pgx.ErrNoRows` case explicitly.

**List pattern with rows.Scan loop (`devices.go` lines 53-73):**

```go
func (s *Store) ListDevicesByUser(ctx context.Context, userID uuid.UUID) ([]Device, error) {
    rows, err := s.pool.Query(ctx,
        `SELECT id, user_id, api_key_id, client_device_id, device_name, os, last_seen_at, created_at
         FROM devices WHERE user_id = $1 ORDER BY last_seen_at DESC`,
        userID,
    )
    if err != nil {
        return nil, fmt.Errorf("listing devices: %w", err)
    }
    defer rows.Close()

    var devices []Device
    for rows.Next() {
        var d Device
        if err := rows.Scan(...); err != nil {
            return nil, fmt.Errorf("scanning device: %w", err)
        }
        devices = append(devices, d)
    }
    return devices, rows.Err()
}
```

**Single-row at-or-before query (Phase 7 `GetSnapshotAt`) — adapt `GetDeviceByClientID` lines 76-90:**

```go
func (s *Store) GetDeviceByClientID(ctx context.Context, clientDeviceID string, apiKeyID uuid.UUID) (*Device, error) {
    var d Device
    err := s.pool.QueryRow(ctx,
        `SELECT id, ... FROM devices WHERE client_device_id = $1 AND api_key_id = $2`,
        clientDeviceID, apiKeyID,
    ).Scan(...)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, fmt.Errorf("getting device by client id: %w", err)
    }
    return &d, nil
}
```

**Adaptation for `GetSnapshotAt(ctx, deviceID, t)`:**

```sql
SELECT id, device_id, captured_at, windows, windows_hash, tier
FROM layout_snapshots
WHERE device_id = $1 AND captured_at <= $2
ORDER BY captured_at DESC
LIMIT 1
```

(RESEARCH.md anti-pattern: never use `WHERE captured_at = $1` because rows are sparse.)

---

### `internal/server/store/layout_snapshots_test.go` (test, testcontainers + pgx)

**Analog:** `internal/server/store/testhelper_test.go` + `internal/server/store/devices_test.go`

**Setup helper (`testhelper_test.go` lines 25-71) — already in tree, shared by all `store_test`:**

```go
func newTestDB(t *testing.T) *testDB {
    t.Helper()
    ctx := context.Background()
    pgContainer, err := postgres.Run(ctx,
        "postgres:16-alpine",
        postgres.WithDatabase("trasker_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).
                WithStartupTimeout(30*time.Second),
        ),
    )
    ...
    if err := runTestMigrations(ctx, pool); err != nil { ... }
    cleanup := func() { pool.Close(); pgContainer.Terminate(ctx) }
    t.Cleanup(cleanup)
    return &testDB{Pool: pool, Cleanup: cleanup}
}
```

**Migration-runner pattern (`testhelper_test.go` lines 74-104) — already discovers `migrations/*.up.sql` automatically. Phase 7's `004_layout_snapshots.up.sql` will be picked up with NO test-helper changes required.**

**Test shape (`devices_test.go` lines 27-52):**

```go
func TestDevices_RegisterAndList(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    tdb := newTestDB(t)
    s, err := store.New(context.Background(), tdb.Pool)
    require.NoError(t, err)

    ctx := context.Background()
    user, key := createTestUserAndKey(t, s, "dev1aa")

    device, err := s.UpsertDevice(ctx, store.UpsertDeviceParams{...})
    require.NoError(t, err)
    assert.NotEmpty(t, device.ID)
    assert.Equal(t, "linux", device.OS)
    ...
}
```

**Adaptation for Phase 7:** create user → create api_key → upsert device → insert snapshot → assert via Get/List. Use `createTestUserAndKey` helper (already in tree, lines 14-25 of `devices_test.go`).

---

### `internal/server/api/layout_handlers.go` (controller, mixed API-key + JWT auth)

**Analog (POST under API-key auth):** `internal/server/api/timesheet_handlers.go`

**Auth context extraction + body decode + validation (lines 12-48):**

```go
func timesheetSubmitHandler(deps *Dependencies) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID, ok := auth.UserIDFrom(r.Context())
        if !ok {
            respondError(w, http.StatusUnauthorized, "not authenticated")
            return
        }
        apiKeyID, ok := auth.APIKeyIDFrom(r.Context())
        if !ok {
            respondError(w, http.StatusUnauthorized, "no API key context")
            return
        }

        var req struct {
            ClientDeviceID string `json:"client_device_id"`
            Entries        []struct {
                Tag        string `json:"tag"`
                StartedAt  string `json:"started_at"`
                ...
            } `json:"entries"`
        }
        if err := decodeJSON(r, &req); err != nil {
            respondError(w, http.StatusBadRequest, "invalid request body")
            return
        }
        if req.ClientDeviceID == "" {
            respondError(w, http.StatusBadRequest, "client_device_id is required")
            return
        }
        if len(req.Entries) == 0 {
            respondError(w, http.StatusBadRequest, "at least one entry is required")
            return
        }
```

**Device resolution + RFC3339 parse loop (lines 50-90):**

```go
device, err := deps.Store.GetDeviceByClientID(r.Context(), req.ClientDeviceID, apiKeyID)
if err != nil {
    if errors.Is(err, store.ErrNotFound) {
        respondError(w, http.StatusBadRequest, "device not registered")
    } else {
        deps.Logger.Error("failed to resolve device", "error", err, "client_device_id", req.ClientDeviceID)
        respondError(w, http.StatusInternalServerError, "failed to look up device")
    }
    return
}

entries := make([]store.CreateTimesheetEntryParams, 0, len(req.Entries))
for _, e := range req.Entries {
    startedAt, err := time.Parse(time.RFC3339, e.StartedAt)
    if err != nil {
        respondError(w, http.StatusBadRequest, "invalid started_at format, use RFC3339")
        return
    }
    ...
}
```

**Response shape (lines 103-107):**

```go
respondJSON(w, http.StatusCreated, map[string]any{
    "id":           ts.ID,
    "submitted_at": ts.SubmittedAt,
    "entry_count":  len(ts.Entries),
})
```

**Analog (GET under JWT auth):** `internal/server/api/device_handlers.go` lines 64-93

```go
func deviceListHandler(deps *Dependencies) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID, ok := auth.UserIDFrom(r.Context())
        if !ok {
            respondError(w, http.StatusUnauthorized, "not authenticated")
            return
        }
        devices, err := deps.Store.ListDevicesByUser(r.Context(), userID)
        if err != nil {
            deps.Logger.Error("failed to list devices", "error", err)
            respondError(w, http.StatusInternalServerError, "failed to list devices")
            return
        }
        ...
        respondJSON(w, http.StatusOK, result)
    }
}
```

**Pagination helper (already in tree):** `internal/server/api/respond.go` lines 36-49 — `parsePagination(r)` returns `limit, offset` from `?limit=&offset=`. Use for `/timeline` if the result set is large.

**Query-param parsing for `?t=ISO`:** there is no exact analog, but follow the pagination shape:

```go
tParam := r.URL.Query().Get("t")
if tParam == "" {
    respondError(w, http.StatusBadRequest, "t query parameter is required (ISO 8601 / RFC 3339)")
    return
}
t, err := time.Parse(time.RFC3339, tParam)
if err != nil {
    respondError(w, http.StatusBadRequest, "invalid t format, use RFC3339")
    return
}
```

---

### `internal/server/api/router.go` mounting (planner edits, single block addition)

**Analog:** `internal/server/api/router.go` lines 84-96 (API-key group) and lines 99-145 (JWT group)

**API-key group (where ingest lives):**

```go
if deps.APIKeyAuth != nil {
    r.Group(func(r chi.Router) {
        r.Use(auth.APIKeyMiddleware(deps.APIKeyAuth))

        r.Post("/devices", deviceRegisterHandler(deps))
        r.Get("/devices", deviceListHandler(deps))
        r.Patch("/devices/{id}", deviceUpdateHandler(deps))

        r.Post("/timesheets", timesheetSubmitHandler(deps))
        r.Get("/timesheets", timesheetListOwnHandler(deps))
    })
}
```

**Adaptation:** add `r.Post("/layout-snapshots", layoutIngestHandler(deps))` inside this same group.

**JWT group (where read endpoints live):**

```go
if deps.JWTIssuer != nil {
    r.Group(func(r chi.Router) {
        r.Use(auth.JWTMiddleware(deps.JWTIssuer))

        // Dashboard data (own)
        r.Get("/timesheets", timesheetListOwnHandler(deps))
        r.Get("/devices", deviceListHandler(deps))
        ...
    })
}
```

**Adaptation:** add `r.Get("/layout-snapshots", layoutAtHandler(deps))` and `r.Get("/layout-snapshots/timeline", layoutTimelineHandler(deps))` inside this group.

---

### `internal/server/api/layout_handlers_test.go` (test, httptest + bcrypted API key)

**Analog:** `internal/server/api/device_handlers_test.go`

**Test bootstrap with real store + bcrypt-hashed API key + bearer header (lines 21-50):**

```go
func setupDeviceTestServer(t *testing.T) (*api.Dependencies, http.Handler, *store.User, *store.APIKey, string) {
    t.Helper()
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    tdb := newTestDBForAPI(t)
    s, err := store.New(context.Background(), tdb.Pool)
    require.NoError(t, err)

    user, err := s.CreateUser(context.Background(), store.CreateUserParams{
        EntraOID: "dev-handler-oid", Email: "devhandler@example.com", DisplayName: "Dev Handler",
    })
    require.NoError(t, err)

    plainKey := "tsk_devicehandlertest1234567890"
    hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
    apiKey, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
        UserID:    user.ID,
        KeyHash:   string(hash),
        KeyPrefix: plainKey[:8],
        ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
    })
    require.NoError(t, err)

    deps := &api.Dependencies{Store: s, APIKeyAuth: api.NewStoreAPIKeyAdapter(s)}
    router := api.NewRouter(deps)

    return deps, router, user, apiKey, plainKey
}
```

**HTTP request shape with bearer auth (lines 52-73):**

```go
func TestDeviceHandler_Register(t *testing.T) {
    _, router, _, _, plainKey := setupDeviceTestServer(t)

    body, _ := json.Marshal(map[string]string{
        "client_device_id": "device-uuid-100",
        "os":               "linux",
    })

    req := httptest.NewRequest("POST", "/api/v1/devices", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+plainKey)
    req.Header.Set("Content-Type", "application/json")

    rr := httptest.NewRecorder()
    router.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusCreated, rr.Code)
    ...
}
```

**Adaptation for Phase 7:** copy `setupDeviceTestServer` verbatim, rename to `setupLayoutTestServer`. The same bcrypt+bearer flow exercises the API-key middleware. For JWT-protected reads, JWT setup is more involved — RESEARCH.md does not specify a JWT test helper exists; planner should check whether one needs adding (look at `auth_handlers_test.go` for current JWT test pattern; if absent, the JWT-read tests may need to be deferred to manual smoke).

---

### `migrations/004_layout_snapshots.up.sql` and `.down.sql` (migration)

**Analog:** `migrations/001_init.up.sql` (lines 1-99) and `migrations/001_init.down.sql` (lines 1-11)

**up.sql header + extension reuse pattern (lines 1-4):**

```sql
-- Trasker initial schema
-- This migration creates all tables needed for the server.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- for gen_random_uuid()
```

(`pgcrypto` is already enabled by 001; do NOT re-add. Just use `gen_random_uuid()` directly.)

**Table + index pattern (lines 32-44):**

```sql
CREATE TABLE devices (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    api_key_id       UUID NOT NULL REFERENCES api_keys(id),
    client_device_id TEXT NOT NULL,
    device_name      TEXT,
    os               TEXT NOT NULL,
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_devices_client_api ON devices(client_device_id, api_key_id);
CREATE INDEX idx_devices_user ON devices(user_id);
```

**Adaptation (planner copies):**

```sql
-- migrations/004_layout_snapshots.up.sql
CREATE TABLE layout_snapshots (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id    UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    captured_at  TIMESTAMPTZ NOT NULL,
    windows      JSONB NOT NULL,
    windows_hash TEXT NOT NULL,
    tier         TEXT NOT NULL DEFAULT 'raw',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_layout_tier CHECK (tier IN ('raw', '10min', '1hr'))
);

CREATE INDEX idx_layout_snapshots_device_captured ON layout_snapshots (device_id, captured_at DESC);
CREATE INDEX idx_layout_snapshots_tier_captured  ON layout_snapshots (tier, captured_at);
CREATE UNIQUE INDEX idx_layout_snapshots_dedup   ON layout_snapshots (device_id, captured_at, windows_hash);
```

**Down pattern (`001_init.down.sql` whole file):**

```sql
-- Trasker initial schema — rollback
-- WARNING: This drops all tables and data.

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS timesheet_entries;
DROP TABLE IF EXISTS timesheets;
...
```

**Adaptation:**

```sql
-- migrations/004_layout_snapshots.down.sql
-- WARNING: This drops all layout snapshot data.
DROP TABLE IF EXISTS layout_snapshots;
```

---

### `deploy/initdb/001_schema.sql` (config, append-only)

**Analog:** the file itself (lines 1-117 already contain 001+002+003 inlined). The pattern is: every numbered migration eventually gets appended verbatim to this single file so that fresh-install Postgres `initdb` brings up the full schema in one shot.

**Append:** the same `CREATE TABLE layout_snapshots ...` block from `migrations/004_layout_snapshots.up.sql` (no changes — same DDL works for both).

---

### `web/server-ui/src/routes/(app)/layout/+page.svelte` (component, page composition)

**Analog:** `web/server-ui/src/routes/(app)/devices/+page.svelte`

**Page-shell structure (lines 64-67) — copy verbatim, change title:**

```svelte
<div class="space-y-6">
  <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Devices</h1>
```

**Card chrome (lines 94-97) — copy for "Today's snapshots" / "Snapshot at {ISO}":**

```svelte
<div class="bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700">
    <div class="px-5 py-4 border-b dark:border-slate-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">Registered Devices</h2>
    </div>
```

**Loading / error / empty / data branch pattern (lines 99-156):**

```svelte
{#if loading}
  <div class="p-5 text-gray-500 dark:text-gray-400">Loading devices...</div>
{:else if error}
  <div class="p-5 text-red-600 dark:text-red-400">{error}</div>
{:else if devices.length === 0}
  <div class="p-8 text-center text-gray-400 dark:text-gray-500">
    No devices registered yet. ...
  </div>
{:else}
  <div class="overflow-x-auto">
    <table class="w-full text-sm">
      <thead class="bg-gray-50 dark:bg-slate-700/50 text-left text-gray-500 dark:text-gray-400">
        ...
      </thead>
      <tbody class="divide-y dark:divide-slate-700">
        {#each devices as device}
          <tr> ... </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
```

**Mount-fetch + error capture (lines 13-21) — for `+page.svelte` if planner skips the `+page.ts` loader. UI-SPEC prescribes a `+page.ts` loader, so only use this if planner deviates:**

```svelte
onMount(async () => {
    try {
      devices = await api.devices.list();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load devices.';
    } finally {
      loading = false;
    }
  });
```

**Adaptation:** UI-SPEC mandates a SvelteKit `+page.ts` `load` function (server-side data fetch + permalink-aware). The `onMount` pattern above is the closest in-tree analog but is deliberately superseded by RESEARCH.md's `url.searchParams.get('t')` example. See "No Analog Found" for `+page.ts`.

---

### `web/server-ui/src/lib/api.ts` (utility, add `layout` namespace)

**Analog:** the `timesheets` namespace at lines 143-159

```ts
timesheets: {
    list(params?: { from?: string; to?: string }): Promise<Timesheet[]> {
      const query = new URLSearchParams();
      if (params?.from) query.set('from', params.from);
      if (params?.to) query.set('to', params.to);
      const qs = query.toString();
      return request('GET', `/timesheets${qs ? '?' + qs : ''}`);
    },
    team(params?: { from?: string; to?: string; user_id?: string }): Promise<Timesheet[]> {
      ...
    },
  },
```

**Adaptation (verbatim from UI-SPEC + RESEARCH.md):**

```ts
layout: {
    timeline(params: { device_id: string; from: string; to: string }):
      Promise<Array<{ id: string; captured_at: string; windows_count: number }>> {
        const q = new URLSearchParams(params).toString();
        return request('GET', `/layout-snapshots/timeline?${q}`);
    },
    at(device_id: string, t: string): Promise<LayoutSnapshot> {
        return request('GET', `/layout-snapshots?device_id=${device_id}&t=${encodeURIComponent(t)}`);
    },
},
```

---

### `web/server-ui/src/lib/types.ts` (model, append two interfaces)

**Analog:** `Device` interface at lines 13-22

```ts
export interface Device {
  id: string;
  user_id: string;
  api_key_id: string;
  client_device_id: string;
  device_name: string | null;
  os: string;
  last_seen_at: string;
  created_at: string;
}
```

**Adaptation:**

```ts
export interface LayoutWindow {
  app_name: string;
  window_title: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface LayoutSnapshot {
  id: string;
  device_id: string;
  captured_at: string;
  windows: LayoutWindow[];
  windows_hash: string;
}
```

---

### `web/server-ui/src/lib/components/Sidebar.svelte` (config, single-line nav addition)

**Analog:** the existing `navItems` entry at line 14:

```ts
{ label: 'Devices', href: '/devices', icon: 'M9.75 17L9 20l-1 1h8...' },
```

**Adaptation (UI-SPEC supplies the icon path verbatim; insert after Devices, before Team):**

```ts
{ label: 'Layout', href: '/layout', icon: 'M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25A2.25 2.25 0 018.25 10.5H6A2.25 2.25 0 013.75 8.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 018.25 20.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25A2.25 2.25 0 0113.5 8.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z' },
```

---

## Shared Patterns

Cross-cutting patterns that apply to multiple Phase 7 files.

### Pattern S-1: API-key auth context extraction (server controllers)

- Source: `internal/server/api/timesheet_handlers.go` lines 14-23
- Apply to: `layout_handlers.go` POST ingest handler
- Excerpt:

```go
userID, ok := auth.UserIDFrom(r.Context())
if !ok {
    respondError(w, http.StatusUnauthorized, "not authenticated")
    return
}
apiKeyID, ok := auth.APIKeyIDFrom(r.Context())
if !ok {
    respondError(w, http.StatusUnauthorized, "no API key context")
    return
}
```

### Pattern S-2: JSON request body decode with size limit

- Source: `internal/server/api/respond.go` lines 27-32
- Apply to: every POST handler in `layout_handlers.go`
- Excerpt:

```go
func decodeJSON(r *http.Request, v any) error {
    r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1MB limit
    defer r.Body.Close()
    return json.NewDecoder(r.Body).Decode(v)
}
```

(The 1MB cap may be tight for layout snapshots if `windows` is large; planner should consider raising to 4MB or 8MB for `/layout-snapshots`. Verify against `maxWindows = 256` × ~200 bytes × overhead = well under 1MB.)

### Pattern S-3: pgx ErrNoRows -> domain ErrNotFound

- Source: `internal/server/store/devices.go` lines 76-90
- Apply to: every server store accessor in `layout_snapshots.go`
- Excerpt:

```go
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    return nil, fmt.Errorf("getting device by client id: %w", err)
}
```

### Pattern S-4: HTTP error response shape

- Source: `internal/server/api/respond.go` lines 22-24 (`respondError`) and lines 11-19 (`respondJSON`)
- Apply to: every handler in `layout_handlers.go`
- Excerpt:

```go
func respondJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if v != nil {
        if err := json.NewEncoder(w).Encode(v); err != nil {
            slog.Error("failed to encode JSON response", "error", err)
        }
    }
}

func respondError(w http.ResponseWriter, status int, message string) {
    respondJSON(w, status, map[string]string{"error": message})
}
```

### Pattern S-5: UTC RFC3339 timestamps in client SQLite

- Source: `internal/client/store/focus_events.go` line 25-29
- Apply to: `internal/client/layout/store.go` (every Insert/Update)
- Excerpt:

```go
now := time.Now().UTC().Format(time.RFC3339)
result, err := s.db.Exec(
    `INSERT INTO focus_events (app_name, window_title, started_at, created_at)
     VALUES (?, ?, ?, ?)`,
    appName, windowTitle, startedAt.Format(time.RFC3339), now,
)
```

### Pattern S-6: HTTP client bearer-token POST + sentinel errors

- Source: `internal/client/sync/client.go` lines 136-174
- Apply to: any POST in Phase 7 layout sync (preferably reuse `sync.Client` rather than instantiating a new HTTP client)
- Excerpt:

```go
func (c *Client) post(ctx context.Context, path string, payload any) ([]byte, error) {
    data, err := json.Marshal(payload)
    if err != nil { return nil, fmt.Errorf("sync client: marshal: %w", err) }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, bytes.NewReader(data))
    if err != nil { return nil, fmt.Errorf("sync client: create request: %w", err) }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+c.apiKey)

    resp, err := c.httpClient.Do(req)
    if err != nil { return nil, fmt.Errorf("sync client: request failed: %w", err) }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    ...
    switch resp.StatusCode {
    case http.StatusOK, http.StatusCreated:
        return body, nil
    case http.StatusUnauthorized:
        return nil, ErrKeyExpired
    case http.StatusForbidden:
        return nil, ErrKeyRevoked
    ...
    }
}
```

### Pattern S-7: structured slog with key-value pairs

- Source: `internal/server/api/router.go` lines 168-176 (request logger middleware) and `internal/client/sync/queue.go` lines 191-198 (warn-level retry log)
- Apply to: every Phase 7 service that logs (capturer, sync, downsampler, ingest handler)
- Excerpt:

```go
q.logger.Warn("submission failed, will retry",
    "submission_id", sub.ID,
    "retry_count", sub.RetryCount+1,
    "error", err,
)
```

### Pattern S-8: SvelteKit dark-mode pairings

- Source: `web/server-ui/src/routes/(app)/devices/+page.svelte` (whole file)
- Apply to: every Phase 7 Svelte component
- Examples:
  - Card: `bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700`
  - Card header: `px-5 py-4 border-b dark:border-slate-700`
  - Body text: `text-gray-700 dark:text-gray-300`
  - Secondary text: `text-gray-500 dark:text-gray-400`
  - Tertiary text: `text-gray-400 dark:text-gray-500`
  - Heading: `text-lg font-semibold text-gray-900 dark:text-white`
  - Page title: `text-2xl font-bold text-gray-900 dark:text-white`
  - Error: `text-red-600 dark:text-red-400`
  - Hover row: `hover:bg-gray-50 dark:hover:bg-slate-700/50`
  - Divider: `divide-y dark:divide-slate-700`
- Note: all of these are already enumerated in 07-UI-SPEC.md but cited here so the planner can copy without re-loading the UI spec.

### Pattern S-9: `if testing.Short() { t.Skip(...) }` for testcontainers tests

- Source: `internal/server/store/devices_test.go` line 28-29
- Apply to: every Phase 7 server-side `*_test.go` (both `internal/server/store/layout_snapshots_test.go` and `internal/server/api/layout_handlers_test.go`)
- Excerpt:

```go
if testing.Short() {
    t.Skip("skipping integration test")
}
```

---

## No Analog Found

Files for which the trasker codebase has no close existing match. Planner falls back to RESEARCH.md / UI-SPEC patterns.

- `web/server-ui/src/routes/(app)/layout/+page.ts`
  - Role: route (SvelteKit `load` function)
  - Data flow: request-response
  - Reason: there are zero `+page.ts` files in `web/server-ui/src/routes/`. Every existing route uses `onMount` inside `+page.svelte` instead. The route group `(app)/+layout.svelte` exists but does NOT contain a `+layout.ts` either.
  - Fallback source: RESEARCH.md "SvelteKit `+page.ts` reading `?t=`" code example (lines 555-585). Cited reference: SvelteKit official docs https://svelte.dev/docs/kit/load.
  - Recommendation to planner: introducing the first `+page.ts` in this codebase is a small but genuine convention shift. Verify with Jaypaul whether to keep `onMount` (matches existing convention; checker may complain about UI-SPEC drift) OR introduce `+page.ts` (matches UI-SPEC as written; sets a new precedent the rest of the dashboard does not yet follow). UI-SPEC currently says `+page.ts`; planner should not silently swap to `onMount` without surfacing the choice.

---

## Metadata

- **Analog search scope:**
  - `internal/client/layout/...` (target — does not yet exist)
  - `internal/client/tracker/` (X11 + cgo + nocgo + interface)
  - `internal/client/store/` (sqlite migrations + accessors + tests)
  - `internal/client/sync/` (Queue + Client + tests)
  - `internal/client/presence/` (screenlock + state machine)
  - `internal/server/store/` (pgx + testhelper)
  - `internal/server/api/` (handlers + router + respond + testhelper)
  - `migrations/` and `deploy/initdb/`
  - `web/server-ui/src/routes/(app)/` and `web/server-ui/src/lib/`

- **Files scanned:** 23 source files read in full (all under 2,000 lines). No re-reads.

- **Pattern extraction date:** 2026-05-07

---

## PATTERN MAPPING COMPLETE

- **Phase:** 7 - Layout Snapshots
- **Files classified:** 23
- **Analogs found:** 22 / 23

### Coverage

- Files with exact analog: 14
- Files with role-match analog: 6
- Files with partial-match analog: 2
- Files with no analog: 1 (`+page.ts` — first SvelteKit loader in tree)

### Key Patterns Identified

- All client X11 cgo code follows the `tracker_linux_x11.go` skeleton: `//go:build linux && cgo` header + cgo block + `XOpenDisplay` constructor returning `(T, error)` + buffer-on-stack `unsafe.Pointer` cgo string return. Phase 7 swaps `XGetInputFocus` for `_NET_CLIENT_LIST_STACKING` + `XQueryTree` fallback + per-window `XTranslateCoordinates` for absolute geometry.
- All client sqlite accessors follow `focus_events.go`: time.Time fields stored as RFC3339 strings, `database/sql` with `?` placeholders, `LastInsertId()` for new rows, `scanner` interface to share scanner code between `*sql.Row` and `*sql.Rows`. Phase 7 sync state lives as a `synced_at TEXT NULL` column rather than a separate outbox table.
- All server pgx accessors follow `devices.go`: `s.pool.QueryRow(...).Scan(...)` for single-row, `s.pool.Query(...)` + `for rows.Next()` for multi-row, `errors.Is(err, pgx.ErrNoRows)` -> `ErrNotFound`, `RETURNING ...` after INSERT/UPDATE for round-trip. `ON CONFLICT (...) DO NOTHING` is the dedup pattern (matches RESEARCH.md retry-dedup recommendation).
- All Chi handlers follow `device_handlers.go` / `timesheet_handlers.go`: closure factory `handler(deps *Dependencies) http.HandlerFunc`, `auth.UserIDFrom`/`auth.APIKeyIDFrom` context extraction, `decodeJSON(r, &req)` body parse, validation early-return, `deps.Store....` call, `respondJSON(w, status, map[string]any{...})` response. `respondError(w, status, "message")` always returns the same `{"error": "..."}` envelope.
- All client sync goroutines follow `sync/queue.go`: `var backoffSchedule = []time.Duration{1m, 5m, 15m, 1h}`, `Start(ctx)` -> goroutine running `ticker := time.NewTicker(5 * time.Minute)` + initial immediate run, `processOne` splits permanent (sentinel error -> stop) from transient (bump retry, log Warn), success path UPDATEs status + logs Info. Phase 7 explicitly does NOT abstract this; it duplicates the schedule and handler code (Three Examples rule — Phase 7 is the second user, not the third).
- All testcontainers tests follow `internal/server/store/testhelper_test.go`: `postgres.Run("postgres:16-alpine", ...)` + `runTestMigrations` already auto-loads every `migrations/*.up.sql`. Phase 7's new migration files require zero test-helper changes.
- All Svelte components follow `(app)/devices/+page.svelte`: `<div class="space-y-6">` page root, card chrome `bg-white dark:bg-slate-800 rounded-lg shadow-sm border dark:border-slate-700`, loading/error/empty/data four-branch `{#if}{:else if}{:else}{/if}`, dark-mode pairs are mandatory on every text/background.

### File Created

`/home/jaypaulb/Projects/gh/trasker/.planning/phases/07-layout-snapshots/07-PATTERNS.md`

### Ready for Planning

Pattern mapping complete. Planner can now reference analog patterns in PLAN.md files. The single un-analogged file (`+page.ts`) is flagged for explicit Jaypaul/planner decision before checker runs.
