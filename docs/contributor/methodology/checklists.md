# Evaluation Checklists

<DocMeta audience="Contributor" status="stable" verified="v0.5.4" />

> Checklists are evaluation gates, split by system boundary so they stay maintainable. A checklist item must be either verifiable in code, in a test run, or against evidence — a checkbox without a mechanism is decoration.
>
> Guardrails and invariants referenced here are defined in [Quality Guardrails](./guardrails.md) and [Platform Design](./platform-design.md). Task work (not evaluation) lives in the [Roadmap](../roadmap.md).

## 1. Core correctness

```text
[x] Application starts against a fresh PostgreSQL database        (test suite bootstrap)
[ ] Migrations complete successfully                             (fresh + existing; experiment D)
[x] Application restarts successfully                            (bootstrap path)
[x] CRUD operations work                                         (api/record tests)
[ ] Transactions are atomic                                      (rollback tests — expand to W-01 class)
[x] Failed writes do not partially persist                       (SAVEPOINT audit guard test)
[ ] Concurrent writes behave correctly                           (concurrency tests — Phase 4 / load harness)
[x] Database constraints are enforced                            (functional unique identity indexes)
[ ] Shutdown releases database resources                         (graceful shutdown test — Phase 0 / reliability tests)
[x] Startup failure is explicit and diagnosable                  (error propagation in Bootstrap)
```

## 2. PostgreSQL

```text
[x] Fresh database works                                          (per-test CREATE DATABASE harness)
[x] Existing database works                                       (upgrade/migration reuse)
[ ] Migration state is reproducible                               (experiment D; reproducible boot)
[ ] Migrations fail safely                                        (experiment D)
[x] Transaction rollback works                                    (db_tx tests)
[ ] Query timeout works                                           (timeout test — Phase 0 / reliability tests)
[ ] Lock timeout works                                            (experiment C / timeout tests)
[ ] Connection acquisition timeout works                          (connect_timeout tests — Phase 0 / reliability tests)
[ ] Pool saturation is bounded                                    (experiment B)
[ ] Pool saturation is observable                                 (wait metrics + alert) ✅ live
[x] PostgreSQL restart is recoverable                             (experiment A)
[ ] Network interruption is recoverable                           (experiment A extended)
[x] TLS configuration works in production mode                    (sslmode warning + prod compose)
[ ] PgBouncer transaction pooling works if supported              (exec/simple_protocol documented;
                                                                end-to-end notes are Phase 4 / PgBouncer)
```

## 3. API contract (reference: [api-contract.md](../../reference/api-contract.md))

```text
[ ] Authentication behavior is compatible                         (Phase 1 / compatibility suite — gap)
[ ] Authorization behavior is compatible                          (compat suite — gap)
[ ] REST endpoints are compatible                                 (compat suite — gap)
[ ] HTTP status codes are compatible                              (compat suite — gap)
[ ] Response structures are compatible                            (compat suite — gap)
[ ] Error structures are compatible                               (compat suite — gap)
[ ] Filtering is compatible                                       (tools/search — gap)
[ ] Sorting is compatible                                         (tools/search — gap)
[ ] Pagination is compatible                                      (compat suite — gap)
[ ] Validation is compatible                                      (forms layer — gap)
[ ] Realtime behavior is compatible                               (realtime tests — gap)
[ ] File behavior is compatible                                   (file endpoint tests — gap)
```

Where possible, every row above is an automated test of external behavior against the API contract — not an implementation snapshot, and not a comparison with another product.

## 4. Security (reference: G-SEC-01…08)

```text
[x] No secrets committed                                         (gitignore; CI secret scan ✅ — gitleaks full history, security-scan.yaml)
[ ] Production secrets supplied securely                         (env/secret store; --encryptionEnv) ✅
[ ] TLS enabled/configured                                       (sslmode require/verify-full; HSTS) ✅
[ ] Database not unintentionally public                          (prod compose internal-only network) ✅
[ ] Metrics not unintentionally public                           (loopback guard + PB_METRICS_EXPOSE) ✅
[ ] Process/container uses least privilege                       (non-root, systemd hardening) ✅
[ ] Rate limiting enabled for sensitive endpoints                (must enable in settings; OTP hardcoded) ⚠️
[ ] Administrative endpoints protected                           (superuser-only groups, SuperuserIPs) ✅
[ ] CORS explicitly configured                                   (--origins, Bearer + AllowCredentials=false) ✅
[ ] Trusted proxy configuration reviewed                         (RealIP spoof warning) ✅
[ ] Encryption key/configuration reviewed                        (PB_ENCRYPTION_KEY mandatory prod) ✅
[ ] Backup/restore operations protected                           (superuser-only; DR trust boundary) ✅
```

## 5. Disaster recovery (reference: [Disaster Recovery](../../deployment/disaster-recovery.md))

```text
[x] Backup can be created                                         (pg_dump path)
[x] Backup failure is observable                                  (error surfaced; backup failure metrics ✅)
[ ] Backup can be stored outside the application host             (S3 wiring implemented)
[ ] Backup can be restored                                        (pg_restore path)
[ ] Restore produces a valid schema                               (experiment E — L3 cross-check)
[ ] Restore preserves application data                            (experiment E)
[ ] Restore preserves authentication data                         (experiment E)
[ ] Restore preserves required files/configuration                (storage + settings)
[ ] Restore procedure is documented                               ([disaster-recovery.md](../../deployment/disaster-recovery.md))
[ ] Restore procedure has been executed successfully              (experiment E — gate)
[ ] Last verified backup is observable                            (last_verified_backup metric — gap)
```

## 6. Release checklist

A release is not ready merely because unit tests pass. The release gate combines this matrix with the [production go-live checklist](../../deployment/production.md#10-go-live-checklist):

```text
[ ] Unit tests pass                                              (go test ./...)
[ ] Integration tests pass                                       (DB-backed suite + -race)
[ ] PostgreSQL compatibility tests pass                          (PG 16/17 matrix — Phase 0 / versioning)
[ ] API compatibility tests pass                                 (Phase 1 / compatibility suite)
[ ] Realtime tests pass                                          (realtime + outbox suites)
[ ] Security checks pass                                         (govulncheck, npm audit, secret scan — security-scan.yaml)
[ ] Migration tests pass                                         (experiment D + migration tests)
[ ] Backup/restore verification passes                            (experiment E + last_verified)
[ ] Failure experiments for affected subsystem pass               (A–E per affected area)
[ ] Observability exists for affected subsystem                   ([observability.md](./observability.md) §5 pairing)
[ ] Documentation updated                                        (docs build + lychee green)
[ ] API contract updated if caller-visible behavior changed      (api-contract.md)
[ ] Rollback/recovery procedure exists                            (upgrade runbook)
```

## 7. Definition of Done (per non-trivial change)

```text
1.  What system property are we changing?
2.  What desired outcome should change?
3.  What invariant must remain true?
4.  Which guard rails apply?
5.  How will we observe the behavior?
6.  How will we test it?
7.  What failure modes are relevant?
8.  What is the acceptance checklist?
9.  What evidence proves it works?
10. What did we learn after implementation?
```

A change is complete only when there is sufficient traceable evidence (PR description answering the 10 questions, artifact in `evidence/` where relevant) — not merely when the code compiles. This is the [required PR template](https://github.com/arief-fajri/pgbase/pulls) content.
