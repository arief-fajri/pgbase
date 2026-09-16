# Contributing and Releasing

<DocMeta audience="Contributor" status="stable" verified="v0.5.2 (923e860)" />

Pointer page. `developing.md` is the single source of truth for commands; `contributing.md` for PR flow. This page is the checklist order.

## 1. Prereqs

Go 1.25+ (CI `>=1.26.5`), Node 22+ (CI `>=25.2.1`; 18 is EOL), PostgreSQL 16+, Docker 24+ optional. Check: `go version`, `node --version`, `psql --version`. (`developing.md` #prereqs. Note: `contributing.md` says Node 24+ — treat `developing.md` as truth.)

## 2. Day-to-day loop

1. Branch from `main` (`git checkout -b my-feature`).
2. Change where it belongs: business logic → `core/`; routes → `apis/`; validation → `forms/`; mail → `mails/`; dashboard → `ui/src/`.
3. Add/update tests (`testing` + `tests.TestApp` harness for DB code). Test DB: `docker compose -f tests/docker-compose.test.yml up -d --wait` (host `:5433`, `PGTEST_*` defaults match; raw core tests need `PB_POSTGRES_*` pointed at the test DB — see `developing.md` #10).
4. Run relevant tests + `make lint` (`golangci-lint run -c ./golangci.yml ./...`).
5. UI change? `cd ui && npm run build` **before** `go build` (binary embeds `ui/dist` via `ui/embed.go`; stale `dist` = stale site).
6. Open PR against `main`. Only PRs and `v*` tags trigger CI (`basebuild`): UI build + test Postgres + `go test ./...` + a parallel `go test -race ./...` job + a `golangci-lint` job (`make lint` equivalent) + 32-bit `GOARCH=arm` check. Docs changes also run the `docs` workflow (markdownlint + VitePress build + lychee link check). A weekly `security-scan` workflow (Monday 06:00 UTC, manual dispatch too) runs `govulncheck` + `npm audit`. Plain `main` pushes run nothing.

## 3. Migrations

System DDL lives in `migrations/` (`16409..._init.go` … `17872..._realtime_outbox_*.go`), each `init()` registering into `core.SystemMigrations`; applied automatically on every start (`core.BaseApp.RunSystemMigrations`, serialized by `pg_advisory_xact_lock` key `11924226342963` in `core/migrations_runner.go:281-307`). Scaffold user migrations: `./pgbase migrate --help` (`plugins/migratecmd`, `--migrationsDir/--automigrate`; JS migrations via `plugins/jsvm`). (`developing.md` #8.)

## 4. Releasing (tag-driven, draft)

Workflow `.github/workflows/release.yaml` + `.goreleaser.yaml` (changelog pipe **enabled**; release body supplied via `--release-notes` from the top `CHANGELOG.md` section; `release.draft: true`):

1. Merge PR (CI green). Update `CHANGELOG.md` with new top `## vX.Y.Z` section (GoReleaser copies the top section as the draft body — must precede the tag).
2. `git checkout main && git pull && git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z` (version string injected at build time from the git tag via `-ldflags -X`; only `v*` triggers).
3. Wait for `basebuild`: tests + GoReleaser cross-platform builds → **draft** release.
4. Review draft under GitHub Releases → Publish.

Mistaken local tag: `git tag -d vX.Y.Z`. Deleting a pushed tag also removes the draft — avoid unless necessary. (`developing.md` #15.)

## 5. Upstream tracking (fork duty)

The fork strategy and its maintenance-cost estimate live in `FORK_STRATEGY.md` at the repository root (hard fork, watch → triage → act, security backport SLA, adopt/skip/diverge rules, revisit triggers). In short: watch upstream releases + Go/npm advisories, triage applicability, backport security fixes within the SLA; per-feature adopt/skip/diverge decisions; scheduled `govulncheck` + `npm audit` gates; compatibility matrix (JS/Dart SDK × PG 16/17) before publishing claims.
