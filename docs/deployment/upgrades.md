# Upgrades & Versioning

<DocMeta audience="Operator" status="stable" verified="v0.5.4" />

How PG-BASE versions break, how to upgrade (and roll back) the app, and how to upgrade PostgreSQL — plus where the supported-engine claim is observed.

> First run & migrations: [Production Runbook §3](./production.md#3-first-run--migrations). Restore procedure: [Disaster Recovery §3](./disaster-recovery.md#3-restore-runbook-operator-executable). Release process for maintainers: [Releasing §4](../contributor/releasing.md#4-releasing-tag-driven-draft).

## 1. How PG-BASE versions

- **Releases** are git tags `vX.Y.Z`, each with a `CHANGELOG.md` section. Release binaries are built by GoReleaser with the version stamped in, so `pgbase --version` prints it (a local `go build` prints `(untracked)`).
- **Pre-1.0 policy.** v0.x carries no stability guarantee beyond the [API contract](../reference/api-contract.md). The contract is the compatibility surface: a caller-visible change ships with a contract edit and a regression test — not with tracking another project's release feed.
- **The binary and the schema move together.** Migrations are append-only: they run in order at every boot (single transaction under a PostgreSQL advisory lock). There is no tool that reverses a migration.

## 2. Upgrading the app

An app upgrade replaces the binary (or image tag) and lets the boot-time migrations bring the schema forward.

1. **Before touching anything:** take a backup and verify it — the verified timestamp is exposed as `last_verified_backup` ([Production Runbook §7](./production.md#7-backup--restore), [Disaster Recovery §4](./disaster-recovery.md#4-verification-discipline)). This backup is your rollback.
2. Note the current version: `pgbase --version`.
3. **Replace the binary:** download the release tag, rebuild from source, or `docker pull ghcr.io/<owner>/pgbase:<tag>`. Prefer immutable tags; `:latest` is only moved after a human publishes the release.
4. **Restart** `pgbase serve`. On boot the migrations runner applies what is pending; a server that is already up to date is a no-op (the advisory lock makes concurrent boots safe).
5. **Verify:** `GET /api/ready` returns 200, then smoke-check the dashboard and one read/write API call.

### Rollback (app)

Migrations are forward-only, so swapping back the old binary **alone is not a supported downgrade** once newer migrations have run. The supported rollback is a restore of the pre-upgrade verified backup:

1. Stop the new binary.
2. Restore the pre-upgrade backup with the **old** binary ([Disaster Recovery §3](./disaster-recovery.md#3-restore-runbook-operator-executable)).
3. Start the old binary and verify `GET /api/ready` returns 200.

## 3. Upgrading PostgreSQL

**Supported engine majors: PostgreSQL 16 and 17.** The claim is observed on every pull request by the `pg-matrix` CI job, which runs the full suite against `postgres:16-alpine` and `postgres:17-alpine` (see [§4](#4-what-ci-observes)). The floor stays 16; 17 is tested, not assumed.

PG-BASE never upgrades PostgreSQL for you — the engine stays your PostgreSQL, with the same `psql` access. Upgrade it with PostgreSQL's own tooling:

1. Take and verify a backup of the application database (as in §2.1).
2. Stop `pgbase serve` (or the whole compose stack).
3. Upgrade the engine with `pg_upgrade` or a logical dump/restore, following the PostgreSQL documentation for your topology.
4. Start `pgbase serve` again and verify `GET /api/ready` returns 200 plus a smoke check.

Tooling note: `pg_dump`/`pg_restore` must be at least the server's major version (client ≥ server), or the native backup path cannot dump the database. CI installs a matching client per matrix leg for exactly this reason.

## 4. What CI observes

| Job | Suite | Engine |
|---|---|---|
| `goreleaser` | plain, full suite | `postgres:16-alpine` |
| `race` | full suite with `-race` | `postgres:16-alpine` |
| `pg-matrix (16)` / `pg-matrix (17)` | plain, full suite | `postgres:16-alpine` / `postgres:17-alpine` |

Run the same legs locally by overriding the test compose image (defaults to 16, so plain `make test` is unchanged):

```bash
TEST_POSTGRES_IMAGE=postgres:17-alpine docker compose -f tests/docker-compose.test.yml up -d --wait
make test
docker compose -f tests/docker-compose.test.yml down -v
```

See [Developing §10](../contributor/developing.md#10-running-tests) for the full test workflow.
