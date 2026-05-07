## Conflict Detection Report

### BLOCKERS (0)

(none)

### WARNINGS (1)

[WARNING] Two SPECs address the same scope (production deployment topology) with divergent technical content
  Found: docs/superpowers/specs/2026-03-23-trasker-design.md (Status: Draft) prescribes a 4-container Docker Compose stack — Caddy (auto-TLS) + Nginx-equivalent SPA host + Go API + Postgres — with the build pipeline cross-compiling client binaries via a separate `Dockerfile.builder` container at request time. docs/superpowers/specs/2026-03-26-production-deployment-design.md prescribes a 2-container stack — Go server (with native `golang.org/x/crypto/acme/autocert` TLS, `go:embed` SPA, and disk-cached pre-compiled client binaries patched via `bytes.Replace`) + Postgres — and explicitly states it "replaces" the 4-container design.
  Impact: Both SPECs are present in the ingest set and neither is `locked`. The newer SPEC self-declares as a replacement, but a downstream synthesizer that merges naively could end up with contradictory deployment guidance (Caddy+Nginx vs autocert; runtime `go build` vs disk-cached patching; `TRASKER_SERVER_URL` env var vs `TRASKER_FQDN`-derived URL; hard-fail JWT secret vs auto-generated `/data/jwt-secret`).
  → Confirm the 2026-03-26 SPEC supersedes the deployment portion of the 2026-03-23 SPEC. Either (a) mark the deployment sections of the older SPEC as Superseded, (b) author a locked ADR recording the pivot, or (c) override per-doc precedence in `--manifest` to give the 2026-03-26 SPEC priority. Until then, treat the 2026-03-26 SPEC as authoritative for deployment content per `context.md` note, and the 2026-03-23 SPEC as authoritative for application/domain content (focus tracking, tagging, note cascade, schema, API contract, etc.) where the two do not overlap.

### INFO (4)

[INFO] No ADRs in ingest set — no LOCKED-vs-LOCKED contradictions possible
  Note: Ingest set is 0 ADR / 0 PRD / 2 SPEC / 8 DOC. Architectural choices observed in SPEC content (e.g. modernc.org/sqlite, monorepo, Entra OIDC, no-input-monitoring) have SPEC-level authority only. They are recorded in `decisions.md` under "Doc-derived directional choices (NOT locked)" and would benefit from being promoted to formal Accepted ADRs before any of them is treated as immovable.

[INFO] No PRDs in ingest set — no competing acceptance variants possible
  Note: User-visible behaviors are derived from SPEC narrative and recorded in `requirements.md` as REQ-* extracts without acceptance-criteria sections. Treat the requirements file as a derivation, not a PRD-equivalent contract — the next planning pass should author formal PRDs with explicit acceptance criteria (especially for the four request-flow REQs: REQ-submit-flow, REQ-presence-detection, REQ-note-cascade, REQ-api-key-lifecycle, all of which have non-trivial acceptance surfaces).

[INFO] Cross-ref graph contains no cycles (and no resolvable cross-doc references beyond DOC→SPEC)
  Note: Of the 10 docs, 4 declare resolvable file-path cross_refs and all of them point at one of the two SPECs (which themselves declare no cross_refs). The remaining cross_refs are human-readable strings like "Plan 01" / "Plans 01-05" — not file paths, so they are not part of the dependency graph. DFS three-color marking found no back edges; max traversal depth observed was 2.

[INFO] Plan 04 (Server Dashboard & Deployment, dated 2026-03-23) overlaps with the 2026-03-26 production-deployment plan
  Note: docs/superpowers/plans/2026-03-23-04-server-dashboard-deploy.md scopes "SvelteKit server-ui, TailwindCSS, Docker Compose, Caddy reverse proxy, PostgreSQL, Go cross-compilation, client build pipeline, deployment" — the deployment + cross-compilation + Caddy portions are obsoleted by the 2026-03-26 SPEC. The SvelteKit / Tailwind / migrations content remains valid. Synthesis preserves the plan as DOC-level context but downstream consumers should split it: the dashboard-build content is current; the deployment content is superseded. This is recorded narratively in `context.md` under "Topic: Server dashboard & deployment (Plan 04)" and "Topic: Architectural pivot to note".
