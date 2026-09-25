# Migrate from PocketBase

<DocMeta audience="App Builder" status="experimental" verified="v0.5.4" />

PG-BASE can import a PocketBase data directory today. A dedicated migration command, a compatibility report, and a verified playbook are **not shipped yet**. Do not treat this page as a finished migration product.

## What works today

PG-BASE restore auto-detects a legacy SQLite `data.db` inside a backup archive and imports it into PostgreSQL.

- Archives from PocketBase `v0.23+` and pre-`v0.23` (`v0.22`) are recognized.
- Base collections are supported. Pre-`v0.23` auth and view collections are best-effort.
- Legacy field options are converted. `created` / `updated` autodate fields are injected. `_admins` are migrated to `_superusers`.
- The import carries records, schema, and settings. It does not carry request logs.
- Restore is offline. Restart the process after `pgbase restore` so the app loads the imported data.

```bash
./pgbase restore your-pocketbase-backup.zip
```

Details: [Disaster recovery](./deployment/disaster-recovery.md). The engine is `core/backup_sqlite_import.go`.

There is no dry-run, no pre/post count report, and no `pgbase migrate --from pocketbase` command.

## What is not claimed yet

A PocketBase JS or Dart app may work against PG-BASE with little or no client change. That is a convenience of the [API contract](./reference/api-contract.md), not a tested compatibility guarantee. There is no published matrix of SDK versions, PocketBase versions, or known behavior gaps beyond that contract.

Read the contract before relying on identity indexes, realtime fanout, batch transactions, or editor HTML.

## What is coming

Phase 1 of the [roadmap](./contributor/roadmap.md) is a repeatable migration:

- `pgbase migrate --from pocketbase` with dry-run and safe failure
- a verification report (counts, relations, auth)
- a compatibility suite for auth, CRUD, filter, sort, expand, relations, realtime, files, rules, and the JS/Dart SDKs
- a playbook that says what must change in application code

Until that ships, import a backup, restart, and check the data yourself.
