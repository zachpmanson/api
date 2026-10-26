# zachmanson.com API

Small standard-library Go HTTP service backed by SQLite. No Node, Prisma, Flask,
PostgreSQL client, or external database is needed at runtime.

## Local development

Use the pinned Nix shell (Go, C compiler, SQLite CLI). CGO is required by the
maintained `github.com/mattn/go-sqlite3` driver; SQLite is compiled into the binary.

```sh
nix develop
# Read a disposable local key without putting it in shell history:
read -rs PROD_BROKE_KEY; echo; export PROD_BROKE_KEY
export API_DB_PATH="$PWD/api.sqlite"
go run ./cmd/api serve
```

Never put real secrets or history in this repository. Go does **not** load `.env`.
Default listener: `127.0.0.1:3000`. Root HTML lists the two public GET endpoints.

```sh
nix develop --command go test ./...
nix develop --command go test -race ./...
nix develop --command go vet ./...
nix build
nix flake check
```

When reviewing an uncommitted worktree with new files, use `path:.` as the flake
argument (`nix develop path:.`, `nix build path:.`, `nix flake check path:.`), since
Git-backed flakes omit untracked files. Do not stage files just for testing.

## HTTP contract

- `GET /api/prod-broke`: JSON array of latest existing event per environment;
  an empty database returns `[]`. Events have exactly `id`, `date`, `deployment`.
- `POST /api/prod-broke`: JSON `{ "deployment": "PROD" | "STG", "date": "…",
  "key": "…" }`. Success: HTTP 200 `{ "success": true, "data": event }`.
  Invalid JSON/fields/dates or bodies over 8 KiB: 400; wrong key: 401.
  Unknown properties are ignored as in the previous Zod schema. Valid JSON is
  accepted even if a legacy caller omits or mislabels request Content-Type.
- Dates: RFC3339 timestamps with `Z` or explicit numeric offsets, or `YYYY-MM-DD`
  (UTC midnight). Calendar-invalid dates are rejected, not rolled over. Ambiguous
  locale strings and timezone-less timestamps are intentionally unsupported.
  Responses are always UTC ISO strings with **three** fractional digits, matching
  Prisma. Sub-millisecond input is truncated. Supported UTC years: 0000–9999.
- `GET /api/prod-broke/messages`: both `PROD` and `STG`, including
  `PROD has never had a broken deployment` / `STG has never had a broken deployment`.
  Elapsed days use floor of UTC elapsed milliseconds / 86400000, not calendar
  boundaries; future events produce negative days, just like JavaScript.
- Array order is `PROD`, then `STG` when present. Equal-date ties choose the
  lexicographically smallest ID (both were unspecified previously).
- `GET /healthz`: 200 `{"status":"ok"}` after a small DB query, otherwise 503.
  GET routes also support HEAD; known routes answer OPTIONS without writing.
  Unsupported methods return 405 with `Allow`; unknown paths return 404.
- `/api/*` retains the previous fixed CORS policy for `https://zachmanson.com`,
  including credentials and preflight headers; arbitrary origins are never
  reflected and no wildcard permission is added. Request/secret/event logging
  is disabled.

## Configuration and hosting

| Environment | Default | Meaning |
| --- | --- | --- |
| `PROD_BROKE_KEY` | none | Shared writer secret; empty/unset prevents startup |
| `API_LISTEN` | `127.0.0.1:3000` | Plain HTTP listener behind the existing HTTPS proxy |
| `API_DB_PATH` | `./api.sqlite` | Persistent SQLite file, never a store path |

CLI: `api [serve|import] [-db PATH] [-listen HOST:PORT] [-file JSON]`.
Import does not require the writer key. A failed import exits nonzero, without
printing records. Runtime errors are deliberately sanitized.

The database uses versioned transactional migrations, WAL, FULL synchronous mode,
a 5-second busy timeout, one connection, constraints and a latest-event index.
Existing files are never cleared or implicitly replaced; unknown schema versions
fail startup. HTTP header/read/write/idle timeouts are 5/10/15/60 seconds; graceful
shutdown permits 10 seconds. The service uses restrictive file permissions.

See [operations and migration runbook](docs/operations.md) for the NixOS module,
secret handling, private history export/import, separate writer-fenced cutover,
rollback/reconciliation, and WAL-safe backup/restore procedures. Host wiring,
HTTPS/DNS, live import and Vercel changes are **not** performed by this repository.

## Legacy reference

[Original Prisma schema](docs/legacy/schema.prisma) is retained only as historical
schema/export documentation, not a runtime dependency. The replaced Next/Prisma
and unused Flask implementations remain in Git history (pre-rewrite commit
`6ecd4391ad66cc02fc8664aac6f5d33f5cd715a7`); do not deploy the Flask service.
All checked-in history fixtures are synthetic.
