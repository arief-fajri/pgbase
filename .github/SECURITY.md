# Security Policy

PG-BASE is a self-hosted, single-binary application backend for PostgreSQL.
We take its security seriously and appreciate responsible disclosure.

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately via **GitHub Private Vulnerability Reporting**:

> Repository **Security** tab → **Report a vulnerability**
> (<https://github.com/arief-fajri/pgbase/security/advisories/new>)

This creates a private advisory visible only to the maintainers and to you.

A short _"I think I found a security issue when I do X"_ is enough to start. Please include
enough detail to reproduce (affected version/commit, steps, and impact).

## Supported versions

Security fixes target the **latest `v0.5.x` release** (see
[Releases](https://github.com/arief-fajri/pgbase/releases)). Older tags are not maintained;
please upgrade to the latest tag before reporting.

| Version   | Supported          |
| --------- | ------------------ |
| `v0.5.x`  | :white_check_mark: |
| `< v0.5`  | :x:                |

## Scope

In scope: the PG-BASE server, its PostgreSQL data/aux layer, the embedded dashboard, the
CLI (`serve`, `migrate`, `backup`, `restore`, `superuser`), the Docker/Compose deployment
assets in this repository, and the GitHub Actions workflows.

Out of scope: your own application data, API rules, and `pb_hooks` scripts (these run with
full trust — see the JSVM note below), and third-party OAuth2 providers you configure.

## What to expect

- We aim to acknowledge a report within a few days. If you do not hear back within a week,
  the notification may have been missed — open a **non-descriptive** GitHub issue simply
  stating that you have a security report waiting so we can reconnect.
- On a confirmed vulnerability we will work on a fix, cut a patched `v0.5.x` release, and
  publish a GitHub Security Advisory (and CVE where warranted) with remediation steps.
- Coordinated disclosure is appreciated: please allow us time to ship a fix before
  publishing details or a PoC.

Dependency advisories are triaged in this repository. See the
[release guide](https://github.com/arief-fajri/pgbase/blob/main/docs/contributor/releasing.md).

## Reports that are usually NOT security issues

The items below are accepted trade-offs. They are generally **not**
treated as vulnerabilities in PG-BASE — but if you can demonstrate concrete impact, we still
want to hear about it.

<details>
<summary><strong>SQL injection via low-level DB methods (e.g. <code>DeleteTable(dangerousName)</code>)</strong></summary>

Raw SQL statements and identifiers (table/column names) passed to the low-level `dangerous*`
helpers are intentionally **not** parameterized and must never receive untrusted input. This
is documented and by design; these methods are superuser/programmatic only.
</details>

<details>
<summary><strong>Race conditions on concurrent record edits</strong></summary>

To reduce DB contention, some read-then-write operations are not wrapped in a single
transaction, which can race if multiple clients edit the same record simultaneously. This is
an accepted trade-off. Use the Batch API or an explicit programmatic transaction when you
need atomicity.
</details>

<details>
<summary><strong>List/Search timing side-channels</strong></summary>

Client-side filterable fields are technically subject to timing analysis. If a field holds
sensitive data (secrets, tokens, codes), mark it **Hidden** so it cannot be used in
client-side filters.
</details>

<details>
<summary><strong>Blind SSRF via a malicious OAuth2 provider</strong></summary>

Uploading an OAuth2 avatar on sign-up assumes you trust the configured OAuth2 vendor. If a
vendor is malicious it can already impersonate your users, so an avatar-URL fetch is not the
primary concern. Only configure OAuth2 providers you trust.
</details>

<details>
<summary><strong>User enumeration</strong></summary>

Some endpoints (e.g. register) can leak account existence via response heuristics. We
minimize this with constant-time checks, non-descriptive errors, and rate limiting, but it
is not fully eliminable without degrading UX.
</details>

<details>
<summary><strong>Attacks relying on social engineering</strong></summary>

Reports that depend on tricking a user (e.g. clicking a crafted link) are usually out of
scope, since many APIs are deliberately designed for low friction.
</details>

<details>
<summary><strong>JSVM (<code>pb_hooks</code>) is not a sandbox</strong></summary>

`pb_hooks` JavaScript runs in-process with full trust — filesystem, network, env vars, and
shell access are all available, exactly like the Go framework. Do **not** run untrusted code
in `pb_hooks`. If you can do the same thing from Go, it is not a JSVM vulnerability.
</details>

<!--
MAINTAINER SETUP (one-time): enable GitHub Private Vulnerability Reporting so the
"Report a vulnerability" button above works —
  Repository Settings → Code security and analysis → Private vulnerability reporting → Enable.
GitHub reporting is currently the only channel; if an email fallback is ever added,
keep it pointed at a monitored address.
-->
