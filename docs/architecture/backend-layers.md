# Architecture: Backend Layers

> Audience: Contributor · Status: stable · Last verified: v0.5.2 (`923e860`)

## 1. Bootstrap chain

```text
examples/base/main.go:20  pgbase.New()
  -> pgbase.go:71-136      NewWithConfig(Config) -> core.NewBaseApp(BaseAppConfig)
  -> examples/base/main.go:119 app.Start()
  -> pgbase.go:147-154     Start() registers cobra cmds, Execute() -> Bootstrap()
  -> cmd/serve.go:41       apis.Serve(app, ServeConfig)
  -> apis/serve.go:74      NewRouter(app) (apis/base.go:19)
  -> apis/serve.go:249-298 OnServe (BuildMux/Listen/Installer) -> Serve/ServeTLS
```

`examples/base/main.go:89-104` also registers the three plugins
(`jsvm`, `migratecmd`, `ghupdate`) and a static `pb_public` route.
The root package is a library — `go run .` fails; the runnable entrypoint is
`./examples/base`. `RunAllMigrations()` runs before listening.

## 2. Layers

| Layer | Path | Role |
|---|---|---|
| CLI | `cmd/serve.go`, `cmd/superuser.go`, `cmd/backup.go`, `pgbase.go` | Flags (`--http/--https/--origins/--dir/--encryptionEnv/--dev/--queryTimeout`); note `--pg-*` flags exist but are **not wired** — connection reads `PB_POSTGRES_*` env only (`../DEV.md` §3) |
| HTTP | `apis/` | Router + handlers + middleware; `Serve()` does migrations → router → CORS → HTTP/HTTPS + autocert, graceful shutdown (`apis/serve.go:159-236`) |
| Domain | `core/` | `App` interface (`core/app.go:29`), `BaseApp` impl (`core/base.go:85`), models, queries, hooks, backups, outbox |
| Libraries | `tools/` (22 pkgs) | `router`, `hook` (priority-sorted chain), `search` (filter/sort), `dbutils` (PG dialect/JSON/index), `auth` (30+ OAuth2 providers), `security`, `subscriptions` (realtime Broker), `filesystem` (local/S3), `archive`, `mailer`, `cron`, `store`, `types`, `template`, `tokenizer`, misc |
| Plugins | `plugins/jsvm`, `plugins/migratecmd`, `plugins/ghupdate` | JS hooks + JS migrations (`--hooksPool/--hooksWatch`), `migrate up/down/history-sync`, HTTPS-only self-update with `checksums.txt` verify |
| Persistence | PostgreSQL via `pgx` + `dbx` | `core/db_connect.go`, `core/db.go`, `core/db_tx.go`, `core/db_retry.go`, `migrations/` (20 system files) |

`BaseApp` fields (`core/base.go:88-105,218-259`): `dataDB` + `auxDB` builders,
`dbConfig`, `store`, `cron`, `subscriptionsBroker`, `logger`,
`realtimeOutboxOrigin`, hook registries, `instanceHeartbeatGuard`.
`IsBootstrapped()` = both pools non-nil (`core/base.go:417`).

## 3. Dual-pool PostgreSQL

- `ResolveDBConfig()` (`core/db_connect.go:37`) fills `DBConfig` from `PB_POSTGRES_*`
  (defaults `localhost:5432/pgbase/prefer`). Warns on non-loopback `sslmode=disable`.
- `DefaultDBConnect()` (`core/db_connect.go:74`): `dbx.Open("pgx", dsn)` +
  `SetMaxOpen/IdleConns`, `SetConnMaxIdleTime 3m`, `SetConnMaxLifetime 30m`,
  `connect_timeout=10` fail-fast dial, optional `default_query_exec_mode`
  (needed for PgBouncer transaction pooling), `search_path` schema support.
- Defaults (`core/base.go:43-46`): data `80/15`, aux `10/3`. Effective
  single-instance ceiling = `80 + 10 = 90` (< stock `max_connections=100`).
  Precedence: `BaseAppConfig` > `PB_POSTGRES_DATA/AUX_MAX_*` > defaults.
- `dataDB` = app traffic; `auxDB` = `_logs` writes (`core/base.go:1525`),
  aux migrations (`core/migrations_runner.go:282`), `AuxVacuum/AuxAnalyze`
  (`core/db_table.go:102,124`) — long-lived work never starves requests.
- Timeouts (layered, `../DEV.md` §3): `DefaultQueryTimeout 30s`
  (`core/base.go:47`; reads via `queryTimeoutHook` in `core/db_retry.go:13`,
  writes via `withWriteDeadline` in `core/db_retry.go:43` — caller context wins);
  server backstop via role-level `statement_timeout 60s` + `lock_timeout 30s`
  (`core/db_connect.go:156-254`, applied `core/base.go:448`);
  `connect_timeout 10s`; recycle via lifetime/idle settings.
- `encryptionEnv` (`pgbase.go:36,52,113,128`, flag `pgbase.go:209-228`):
  names the env var holding the 32-char settings-encryption key
  (`core/settings_model.go:273`).

## 4. Middleware execution order (priority, not bind order)

`apis/base.go:30-36` bind order matches the old (wrong) doc. Execution is
`Priority`-sorted (`tools/hook/hook.go:98-101`, lower runs first):

```text
CORS(-1041, apis/serve.go:79-82; preflight first)
  -> ActivityLogger(-1040, apis/middlewares.go:35)
  -> PanicRecover(-1030, :40)
  -> LoadAuthToken(-1020, :43; populates e.Auth)
  -> SuperuserIPsWhitelist(-1015, :46; only with superuser auth, :338-354)
  -> SecurityHeaders(-1010, :49; nosniff/SAMEORIGIN/Referrer, HSTS iff PB_HSTS)
  -> RateLimit(-1000, apis/middlewares_rate_limit.go:14; per-instance store+cron :162,219)
  -> BodyLimit(-990, apis/middlewares_body_limit.go:16; default 32 MiB)
```

Plus: `wwwRedirect(-99999)`, Gzip on `/api` (`apis/base.go:46`, min 1024B;
opt-outs via `Unbind` in `apis/realtime.go:40` SSE, `apis/file.go:47`,
`apis/backup.go:24`; UI `/_/` gzip in `apis/serve.go:124`),
per-route `RequireAuth/GuestOnly/SuperuserAuth/SuperuserOrOwnerAuth/SameCollectionContextAuth`,
per-collection `collectionPathRateLimit/dynamicCollectionBodyLimit`,
opt-in `metricsMiddleware` on the separate `/metrics` listener.

## 5. Migrations runner

`core/migrations_list.go`: `Migration{Up/Down, File}` in `SystemMigrations` +
`AppMigrations`, sorted by filename. `core/migrations_runner.go`:
`MigrationsRunner{table=_migrations}`, `Up()` in tx, `pg_advisory_xact_lock(11924226342963)`
serializes concurrent instances; per-job cron guard uses non-blocking
`pg_try_advisory_lock` on a dedicated conn (`core/cron_guard.go:33-62`).
Sources: compiled `migrations/*.go` + user `pb_migrations/` (Go) + JS via `jsvm`
(`MigrationsDir=DataDir/../pb_migrations`, `plugins/jsvm/jsvm.go:94,137,220-222`).
