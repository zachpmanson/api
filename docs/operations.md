# Operations and migration runbook

This document is a procedure for **separately approved** operator work. The
implementation worktree does not authorize production connections, live export,
import, writer freeze, deployment, Vercel changes, DNS or cutover. The reported
18 PostgreSQL rows are a point-in-time owner inventory, not a fixed importer
assumption. No production records or credentials are checked in.

## NixOS integration (host repository, separately reviewed)

The flake exposes `packages.<system>.default` (`bin/api`) and
`nixosModules.default`. A later host change can use:

```nix
{
  imports = [ inputs.api.nixosModules.default ];
  services.deployment-tracker = {
    enable = true;
    # package = inputs.api.packages.${pkgs.stdenv.hostPlatform.system}.default;
    listenAddress = "127.0.0.1:3000";
    environmentFile = config.age.secrets.api-prod-broke.path;
  };
  # Define age.secrets.api-prod-broke in the host repository, not here.
}
```

The environment file contains `PROD_BROKE_KEY=...`, with systemd EnvironmentFile
quoting if needed. Keep it root-owned, mode 0600; agenix can provide its runtime
path under `/run/agenix`. The system manager reads it before dropping privileges.
**Use a string/runtime secret path, not a Nix path literal, `builtins.readFile`,
or a key embedded in Nix configuration.** The module rejects store/relative paths.
An unset, empty, or unavailable secret prevents startup. Never put the key in
ExecStart, command-line flags, shell history, access logs or diagnostic dumps.

The dedicated `deployment-tracker` system user/group owns
`/var/lib/deployment-tracker` (systemd `StateDirectory`, mode 0700). The database
is `/var/lib/deployment-tracker/history.sqlite`, not in `/nix/store`. The module
has no public port/firewall, reverse proxy, DNS or PostgreSQL configuration.
Networking is restricted to localhost by systemd; an existing HTTPS proxy must
terminate TLS. The existing fixed `https://zachmanson.com` CORS policy is retained;
no wildcard or arbitrary-origin reflection is added. Keep request-body logging disabled at the
proxy too, and configure a matching small request-body limit/timeouts there.

Hardening includes a read-only system, private temporary/devices namespaces,
protected home/kernel/control groups, no capabilities/new privileges, restricted
syscalls/address families, and a 0077 umask. `StateDirectory` is writable despite
`ProtectSystem=strict`. Test the sandbox on Naboo before approving real traffic;
module evaluation is not an actual systemd boot or deployment test.

## Local/offline acceptance first

```sh
nix develop path:. --command go test ./...
nix develop path:. --command go test -race ./...
nix develop path:. --command go vet ./...
nix build path:. --no-link
nix flake check path:.
```

`nix build` runs all Go tests. The flake's Linux module check evaluates dedicated
ownership, state path, loopback defaults, and acceptance/rejection of runtime vs
store/relative secret paths. Import tests use **synthetic** IDs/data only.
`path:.` is needed only until new files are committed; it avoids staging for Nix.
Use private disposable files outside the repository for ad-hoc database tests.

## Private history export (requires independent approval)

1. Confirm the owner-approved source, authorization and schema. Use a read-only
   database role; inspect column type and verify legacy Prisma timestamp values
   are UTC. `docs/legacy/schema.prisma` defines `BrokenDeployment(id,date,deployment)`
   and `PROD`/`STG`. There is no runtime Postgres dependency.
2. Prepare private directory/files outside every repository, mode 0700/0600,
   `umask 077`. Use local `psql` only for this approved export; never connect during
   implementation tests. Set up a private libpq service definition and password
   file using the owner's normal secret handling. Use TLS certificate verification
   (`sslmode=verify-full`, trusted CA) appropriate to the authorized source.
   Do **not** put passwords or connection URIs in arguments or shell history.
3. Example shape, after approval, using a preconfigured private `api-history-export`
   service (the name is not a connection string):

   ```sh
   umask 077
   export PGSERVICEFILE=/private/operator/pg_service.conf
   export PGPASSFILE=/private/operator/pgpass
   export PGSERVICE=api-history-export
   psql --no-psqlrc --quiet --tuples-only --no-align \
     --set=ON_ERROR_STOP=1 --file=docs/export-history.sql \
     > /private/operator/history.json
   ```

   Check the exit code before using the file; a failed export can leave an empty
   or partial output file. Do not print it into logs, issues, chat or CI artifacts.
   The checked-in SQL performs only a read-only consistent-snapshot transaction;
   its result is a **JSON array**, not JSON-lines, CSV, or a Prisma dump.
4. Privately validate the array length against a separately recorded count in the
   same frozen source, IDs, environments, date ranges, and latest per environment.
   Final count may differ from the earlier 18-row inventory. A preliminary export
   is not a final snapshot until the old writer is fenced.

## Import: offline first, then separately approved target operation

Format (only synthetic illustration):

```json
[
  {"id":"11111111-1111-4111-8111-111111111111","date":"2024-06-01T00:00:00.000Z","deployment":"PROD"}
]
```

Use the built `bin/api` or `nix run . -- import ...`. `api import -db PATH -file
PRIVATE_JSON` needs no shared key or Postgres credentials. For the service DB,
execute the reviewed package **as `deployment-tracker`**, with access to a private
staged input file owned by that account. Do not run an importer as root against a
service-owned database (root-owned WAL/SHM files could break later writes).
Stop the target service for final import and snapshot verification; do not expose
it to external writers yet. A private example command on the approved target is:

```sh
# <reviewed-api-package> is the pinned built package path, not a secret.
sudo -u deployment-tracker <reviewed-api-package>/bin/api import \
  -db /var/lib/deployment-tracker/history.sqlite \
  -file /var/lib/deployment-tracker/private-history.json
```

The private input must not be reachable over HTTP or backed up to a public
location. Delete staged exports only per the owner's retention policy.

Importer guarantees:

- At most 16 MiB; an array of objects with `id`, `date`, `deployment`, no unknown
  properties. All fields required; valid UUID strings, supported dates, `PROD`/`STG`.
- UTC millisecond date normalization; original ID string and environment retained.
  Legacy timestamp(3) exports lose no precision. Higher precision is truncated,
  just like the public API, so do not use this format for unrelated data.
- Full validation before inserts, then **one transaction**. Any conflict or DB error
  rolls back all new rows in that import, including rows before the conflict.
- An existing ID with identical instant/environment is a no-op; repeated imports
  are safe. An ID with a different instant/environment is a hard failure. Duplicate
  IDs within a single file are rejected, even identical duplicates.
- Existing target-only rows are retained. No reset, overwrite, truncation, implicit
  restore, or replacement of history is performed. Empty arrays are harmless.

Before exposing traffic, independently verify `PRAGMA integrity_check`, row count,
all IDs/date/environment values, and latest results against the private source.
Record private checksums/counts for review, not event data in public artifacts.
Re-run the import to verify zero inserted rows. GET is only a latest-view and is
**not** a full-history reconciliation tool.

## Writer-fenced cutover (not part of implementation)

1. Obtain owner approval for final export/import and **separate approval for
   Vercel writer freeze and routing/cutover**. Keep DigitalOcean/Vercel intact.
2. Identify every writer/client, including scheduled jobs. Fence old writes at the
   old ingress/application or revoke the old writer credential under an approved
   plan. Account for in-flight requests; DNS changes alone do not fence writes.
   Privately confirm old writes stop. Do not send test writes to production.
3. Take the final read-only source snapshot after the fence. Keep target external
   writes disabled. Back up any existing target history, then import transactionally
   and compare the full history. Re-importing an earlier snapshot does not delete
   target-only rows, but cannot capture source writes after that snapshot.
4. Separately reviewed host wiring starts the loopback service and tests health,
   read endpoints, state permissions and restarts. Separately approved proxy/DNS
   changes route clients. Re-enable only the intended target writer and monitor
   sanitized service status/error counters. The same shared key must remain usable
   by the intended clients; coordinate any rotation with the writer-fencing plan.
5. Keep old writer fenced and source retained until owner sign-off and backup
   verification. This repository does not automate any of these live steps.

### Rollback / reconciliation

Before any target write, rollback may restore routing to the retained source only
under approval, while ensuring exactly one writer. After **any** target write,
there is no safe blind rollback: fence both writers, preserve full snapshots of
both databases, reconcile IDs/instants/environments and target-only rows through a
reviewed process, verify totals, then choose a single authoritative writer and
reopen it. Never replace SQLite with the old export or reactivate Vercel ignoring
new Naboo events. This importer intentionally cannot resolve conflicting IDs.

## Backup (SQLite WAL-aware)

A live database can have committed rows in `history.sqlite-wal`. **Never copy just
the live main file**, and do not assume sequential copies of DB/WAL/SHM are atomic.
Use SQLite's online backup API (`sqlite3 .backup`) or stop the service cleanly and
use an approved consistent backup procedure. Backups contain private history:
restrict access, encrypt off-host storage, set retention and test restores.

Example operator procedure, as the service user, after creating a private backup
directory owned by that user (0700); filenames below are fixed local examples:

```sh
umask 077
sqlite3 /var/lib/deployment-tracker/history.sqlite <<'SQL'
.timeout 5000
.backup '/var/lib/deployment-tracker/backups/history-backup.sqlite'
SQL
# Check command exit status, then check the independent backup:
sqlite3 /var/lib/deployment-tracker/backups/history-backup.sqlite 'PRAGMA integrity_check;'
```

`.backup` reads a consistent snapshot including committed WAL data; it does not
require a manual checkpoint. Use unique backup names, never the active database
as destination, and keep multiple generations. The app devshell provides the CLI;
host operators must supply a trusted SQLite CLI separately (the module does not
install a Postgres client or backup daemon).

### Restore (explicit, service stopped)

1. Fence writes and stop `deployment-tracker.service`; verify it is stopped. Keep
   the pre-restore state as a recovery snapshot, including main DB and any WAL/SHM
   files now that no process is accessing them. Do not delete or overwrite it.
2. Verify the selected backup's integrity and expected schema/data in a private
   staging location. Restore it as a **new main file** on the same filesystem,
   owned by `deployment-tracker:deployment-tracker`, mode 0600; preserve the state
   directory's 0700 permissions. Move old main/WAL/SHM aside together, then
   atomically rename the staged restored main into `history.sqlite`. Never attach
   stale WAL/SHM files to a different main file.
3. Start the service under the reviewed unit; test health/read endpoints and
   persistence. Reconcile any writes newer than the backup before reopening
   writers. A restore is explicitly destructive of newer state unless reconciled;
   it is not an importer operation or an automatic app recovery mechanism.

## Routine checks / constraints

`GET /healthz` only reports DB reachability, not disk capacity or full integrity.
Monitor unit state/restarts, free disk, private backup success/restore tests, and
proxy failures without logging bodies or keys. Run integrity checks on snapshots
periodically. The single SQLite connection is intentional for this tiny workload;
WAL and a 5-second busy timeout handle competing backup/import writers. Busy,
full-disk or database failures return sanitized 500 responses, never fake success.
A write's network response can be lost after commit: API POST has no idempotency
key (same as the legacy API), so clients must not assume blind retries are unique.

See [validation evidence](validation.md) for local-only test/build/RAM measurements
and their limitations. No measured comparison with the previous production app is
claimed.
