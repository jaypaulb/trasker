## Pre-existing build failure outside Plan 07-02 scope

- File: `internal/client/webui/embed.go:9`
- Symptom: `pattern all:static: no matching files found` — the embed
  directive references a `static/` directory that does not exist on the
  base commit (`7154943`).
- Verified pre-existing by checking out `embed.go` from the base commit
  in isolation; it fails identically. Not introduced by 07-02.
- Effect on this plan: `go build ./...` and `CGO_ENABLED=0 go build ./...`
  fail at this file; the `internal/client/layout/...` package itself
  builds cleanly under both modes and all 23 unit tests pass.
- Action: out of scope per Plan 07-02 acceptance boundaries. A future
  plan that owns the webui package must restore the `static/` build
  artifacts (likely produced by `web/server-ui` SvelteKit build).
