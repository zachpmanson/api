# Local validation evidence

Implementation and disposable validation used the isolated `fox/go-sqlite`
worktree, based on actual default `origin/main`, commit
`6ecd4391ad66cc02fc8664aac6f5d33f5cd715a7`, for
[zpm/api#1](https://github.com/zachpmanson/api/issues/1).
No production connections, live secrets/history access, deployment or Vercel/DNS
changes were performed. Publishing the implementation is separate from deployment.

## Commands and results

Using the pinned Nix environment on Linux x86_64 (Go 1.26.8; shell SQLite CLI
3.51.2):

- `nix develop path:. --command bash -c 'gofmt -w cmd internal; go mod tidy;
  go test ./...; go test -race ./...; go vet ./...'`: all passed.
- `nix develop path:. --command go test -cover ./...`: passed; tracker package
  91.7%, CLI package 46.9%. Actual serve/start/restart/shutdown also exercised
  manually below; coverage is not a claim of exhaustive correctness.
- `test -z "$(gofmt -l cmd internal)"` in the shell: passed.
- `nix build path:. --no-link --print-out-paths`: passed. Build runs **both** Go
  test packages. The initial fake vendor hash failed as intended, then was
  replaced by the verified `sha256-IUyZTlMqht5sx010M3kc96okTBsRrL1e+6m1VVWvK0k=`.
- `nix flake check path:. --no-write-lock-file`: all current-system checks passed,
  including NixOS module evaluation/secret-path assertions. An initial test
  harness evaluated unrelated lazy NixOS assertion messages; corrected the harness
  to evaluate assertion booleans. No service deployment was used to test it.
- `git diff --check`: passed. `git diff --cached --exit-code`: passed, no staged
  changes. `nix flake lock` initially auto-staged its new lock; it was immediately
  unstaged while retaining the file. No implementation files were staged.

Automated cases cover empty/latest response shapes, UTC millisecond dates,
offsets/date-only inputs and calendar/offset rejection, UUID generation, auth,
malformed/oversized bodies, required exact-case fields, ignored extra fields,
method/response types, fixed-origin CORS/preflight, HEAD, legacy request-header
tolerance, error responses, elapsed/future day flooring,
never-broken messages, deterministic ties/order, constraints/index/schema version,
WAL/busy configuration, parallel reads/writes and cross-connection writer wait,
restart persistence, fixture import/repeat/date equivalence, date/environment
conflicts, malformed/oversized/duplicate import files, rollback and no overwrites.

## Built-binary smoke and memory observation

A shell inside `nix develop path:.` ran the Nix-built binary with a disposable
local-only key, a private `mktemp` directory, and `127.0.0.1:39871`. It:

1. Polled health (first poll raced startup, then 200 `{"status":"ok"}`).
2. Posted five **synthetic** PROD events and performed 200 sequential message GETs.
3. Ran SQLite `.backup` while the service was running. Independent backup returned
   `PRAGMA integrity_check` = `ok`, with all five rows present.
4. Sent SIGTERM, waited for clean exit, restarted with the same file, and verified
   latest date `2024-06-05T00:00:00.000Z` and row count 5 remained.
5. Ran offline CLI import twice on another disposable DB: `3 new rows`, then
   `0 new rows`. Unsetting the shared key rejected serve before creating a DB.
6. Removed all disposable state and stopped every local test process.

Observed `/proc/<pid>/status` VmRSS and VmHWM were both **9868 KiB** after health,
and **11808 KiB** after the five writes + 200 reads. The stripped Nix binary was
8,152,112 bytes. These are one local process's resident/high-water readings, not
PSS, cgroup memory, sustained-load testing, or a production estimate; page cache,
shared mappings, Go GC timing and workload affect the result. **No old Next.js
baseline or measured comparative memory saving is claimed.**

## Remaining gates

Independent review found missing legacy CORS headers; the fixed origin policy,
preflight and HEAD coverage were restored before publication. Review also noted
the documented narrowing to unambiguous RFC3339/date-only inputs, which must be
checked against actual callers before cutover. Remaining: actual Naboo systemd sandbox/HTTPS
integration in the separate host repository; private final export and full-history
comparison; separately approved writer freeze/import/cutover; restore exercise
and reconciliation planning. Only x86_64-linux was built/run here; other flake
systems were not built. No production data was used in tests.
