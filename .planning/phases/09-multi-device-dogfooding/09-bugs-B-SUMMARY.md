---
phase: 09
plan: bugs-B
subsystem: client/sync, client/store
tags: [bug, sqlite, auth, sync, focus-events, modernc-sqlite]
requires: []
provides:
  - store.CloseLatestOpenFocusEvent (package func over *sql.DB)
  - sync.ErrKeyInvalid sentinel
  - sync.ErrPermissionDenied sentinel
  - sync.ClassifyAuthError(body, status) helper (exported)
  - layout.ErrKeyInvalid (re-export)
  - layout.ErrPermissionDenied (re-export)
affects:
  - cmd/trasker-client/main.go (delegates closeCurrentFocusEvent to store)
  - internal/client/sync/queue.go (extends permanent-error set)
  - internal/client/layout/sync.go (postBatch reuses ClassifyAuthError)
tech-stack:
  added: []
  patterns:
    - "subquery form of UPDATE for modernc.org/sqlite (no SQLITE_ENABLE_UPDATE_DELETE_LIMIT)"
    - "wire-format error classification helper as single source of truth"
key-files:
  created: []
  modified:
    - cmd/trasker-client/main.go
    - internal/client/store/focus_events.go
    - internal/client/store/focus_events_test.go
    - internal/client/sync/client.go
    - internal/client/sync/client_test.go
    - internal/client/sync/queue.go
    - internal/client/layout/sync.go
    - internal/client/layout/sync_test.go
decisions:
  - "Fix Bug 2 with a subquery UPDATE rather than ORDER BY+LIMIT — modernc.org/sqlite is not built with SQLITE_ENABLE_UPDATE_DELETE_LIMIT and rejects the latter as a syntax error. Subquery form is portable across all SQLite builds."
  - "Extract close logic into store.CloseLatestOpenFocusEvent(db, now) rather than a *Store method — main.go holds the raw *sql.DB (not a *Store wrapper) and a package-level helper avoids forcing main.go to refactor its DB ownership."
  - "Export ClassifyAuthError so layout/sync.go can reuse the same wire-decoding logic instead of duplicating the if-then-else chain — single source of truth for auth-error mapping (matches the existing 'shared sentinel set' rationale already documented in layout/sync.go)."
  - "Conservative fallback: any 401 with empty/non-JSON body maps to ErrKeyInvalid (not ErrKeyExpired). The previous default lied to the user; the new default at least correctly refuses to claim 'expired' without evidence."
  - "Case-insensitive substring match on the server's `error` field rather than exact-string equality — robust against minor server-side wording changes ('API key has expired' vs 'API key expired' both map to ErrKeyExpired)."
metrics:
  duration: ~25 minutes
  completed: 2026-05-08
---

# Phase 09 Plan bugs-B: Client-Side Bug Fixes Summary

Fixed two client-side bugs surfaced by Phase 7 smoke: a SQLite syntax-error that prevented focus events from ever being closed by the daemon, and a sync-client error-mapping that lied to the user about the cause of every authentication failure.

## Bug 2: focus_events ORDER BY syntax error

### Symptom

Daemon logged `failed to close focus event ... near "ORDER": syntax error` on every focus tick, screen lock, and deadman fire. The result: open focus events accumulated in the SQLite store, none of them ever got an `ended_at` or `duration_s`, and downstream timesheet aggregation saw open-ended rows.

### Root cause

`cmd/trasker-client/main.go::closeCurrentFocusEvent` used:

```sql
UPDATE focus_events
   SET ended_at = ?, duration_s = CAST((julianday(?) - julianday(started_at)) * 86400 AS INTEGER)
 WHERE ended_at IS NULL
 ORDER BY id DESC LIMIT 1
```

`UPDATE … ORDER BY … LIMIT` requires SQLite to be built with `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`. The pure-Go `modernc.org/sqlite` driver does NOT include that compile-time flag, so the parser rejects `ORDER` outright. CGO-based `mattn/go-sqlite3` accepts it; modernc does not. The bug was invisible in any test that mocked the DB or used a different driver.

### Fix

Rewrote the UPDATE as a subquery (portable across all SQLite builds, no compile-time flag required):

```sql
UPDATE focus_events
   SET ended_at = ?, duration_s = CAST((julianday(?) - julianday(started_at)) * 86400 AS INTEGER)
 WHERE id = (
     SELECT id FROM focus_events
      WHERE ended_at IS NULL
      ORDER BY id DESC
      LIMIT 1
 )
```

Extracted into `store.CloseLatestOpenFocusEvent(db *sql.DB, now time.Time) (int64, error)` — a package-level helper because `main.go` holds the raw `*sql.DB`, not a `*Store`. `main.go::closeCurrentFocusEvent` now delegates to the helper.

### Tests

Three regression tests in `internal/client/store/focus_events_test.go`:

- `TestCloseLatestOpenFocusEvent_OnlyClosesOpenRow` — 1 closed row + 1 open row, asserts only the open row is touched, with the correct `duration_s` (300s for a 5-minute span), and the closed row is byte-for-byte unchanged.
- `TestCloseLatestOpenFocusEvent_NoOpenRow` — no-op when there's nothing to close (returns 0 rows affected, no error).
- `TestCloseLatestOpenFocusEvent_ClosesOnlyMostRecentOpen` — when multiple open rows exist (a state we don't intentionally produce, but want to be robust against), only the highest-id row is closed.

All run against the real `modernc.org/sqlite` driver via `clientstore.New()` so the regression cannot recur silently.

### Commit

`1c386d4` — `fix(09-bugs-B): close focus event without ORDER BY+LIMIT in UPDATE`

## Bug 3: sync client error mapping

### Symptom

Any HTTP 401 from the trasker server caused the client to surface `ErrKeyExpired` ("API key expired — download a new client from your Trasker dashboard") regardless of the actual error. Real causes — missing Authorization header, invalid header format, malformed key, bcrypt mismatch on a perfectly-valid-but-wrong key — were all reported as "expired", sending users on a wild goose chase to download a new client when the real fix could be (e.g.) re-running setup or restarting the daemon.

### Root cause

`internal/client/sync/client.go::post()` had a status-only switch:

```go
case http.StatusUnauthorized:
    return nil, ErrKeyExpired
case http.StatusForbidden:
    return nil, ErrKeyRevoked
```

The trasker server's auth middleware (`internal/server/auth/apikey.go`) emits at least five distinct `{"error": "..."}` bodies on 401 — `"missing Authorization header"`, `"invalid Authorization header format"`, `"invalid API key format"`, `"invalid API key"` (returned for both prefix-lookup miss and bcrypt mismatch), `"API key has been revoked"`, and `"API key has expired"` — but the client discarded the body and used only the status code.

### Fix

Added two new sentinels and a shared classifier:

- `ErrKeyInvalid` — 401 where the cause is not "expired" or "revoked" (missing/invalid header, malformed key, bcrypt mismatch).
- `ErrPermissionDenied` — 403 where the cause is not an explicit revoke.
- `ClassifyAuthError(body []byte, status int) error` — exported helper performing case-insensitive substring matching on the body's `error` field, with conservative fallbacks (any unparseable 401 → `ErrKeyInvalid`; any unparseable 403 → `ErrPermissionDenied`).

Wired through:

- `client.go::post()` — switch on 401/403 now calls `ClassifyAuthError`.
- `layout/sync.go::postBatch` — same call (single source of truth, no duplicated if/else chain).
- `queue.go::processOne` and `layout/sync.go::ProcessPending` — both extend their permanent-error checks to include all four sentinels (retrying with the same key cannot succeed for any of them).
- `layout/sync.go` re-exports `ErrKeyInvalid` + `ErrPermissionDenied` to match the existing "shared sentinel set" pattern.

### Tests

`internal/client/sync/client_test.go::TestClient_AuthErrorClassification` is a 12-case table-driven test covering every error message the server actually emits plus edge cases (uppercase "EXPIRED" still matches, empty body falls through to the conservative default, 403 with "revoked" message wins over status). `TestClassifyAuthError_Direct` exercises the helper directly so the layout syncer's reuse path is also covered.

`internal/client/layout/sync_test.go::TestSync_AuthErrorClassification` is a 5-case matrix asserting the layout syncer surfaces the same four sentinels and that all four trigger permanent-error behavior (retry NOT bumped, `synced_at` stays NULL). Existing `TestSync_KeyExpired`/`TestSync_KeyRevoked` still pass because they assert behavior (retry-not-bumped + sentinel re-export equality), not the specific sentinel returned for an empty 401/403 body.

### Commit

`0909f9e` — `fix(09-bugs-B): classify sync auth errors from server body, not just status`

## Verification

```text
go test ./internal/client/store/... ./internal/client/sync/... ./internal/client/layout/...
ok  	github.com/jaypaulb/trasker/internal/client/store	2.472s
ok  	github.com/jaypaulb/trasker/internal/client/sync	0.017s
ok  	github.com/jaypaulb/trasker/internal/client/layout	1.879s
```

`go vet` clean for all three packages. `cmd/trasker-client` `go vet` clean.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Pre-existing missing static directory in webui package**

- **Found during:** Initial `go build ./...` to verify Bug-2 fix compiled.
- **Issue:** `internal/client/webui/embed.go` declares `//go:embed all:static` but `internal/client/webui/static/` does not exist in the repo, so a full-tree `go build` fails.
- **Resolution:** Out of scope for this plan (the bug pre-dates these commits and lives in an unrelated package). Worked around the issue locally by building only the affected packages (`go build ./internal/client/store/... ./internal/client/sync/... ./internal/client/layout/...`) and using `go vet ./cmd/trasker-client/...` (which does not require linking) to verify `cmd/trasker-client` compiles. No file change made; the pre-existing webui build break is logged for a separate cleanup.

### Auth Gates

None — fully autonomous execution.

## Self-Check: PASSED

- All 8 modified files exist on disk.
- Both commits (1c386d4, 0909f9e) present in `git log --oneline --all`.
- `go test ./internal/client/store/... ./internal/client/sync/... ./internal/client/layout/...` all green.
- `go vet ./internal/client/sync/... ./internal/client/layout/... ./cmd/trasker-client/...` clean.
