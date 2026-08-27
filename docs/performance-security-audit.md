# Performance & Security Audit Report

Tanggal audit: 2026-08-27  
Repository: `github.com/arief-fajri/pgbase`  
Metodologi: static code review berbasis OWASP ASVS/API Security Top 10, PTES, NIST SP 800-53/800-218 SSDF, CVSS v3.1 qualitative scoring, dan pemeriksaan dependency/build yang dapat dijalankan di lingkungan ini.

## Executive summary

Audit menemukan beberapa kontrol keamanan yang sudah baik: endpoint SQL dibatasi superuser, request body memiliki limit default, OAuth2 avatar download menggunakan HTTP client dengan pemeriksaan IP internal, TLS minimum diset ke TLS 1.2, dan API memiliki rate-limit framework. Namun terdapat beberapa risiko produksi yang perlu diprioritaskan:

1. **High**: runtime JavaScript hooks mengekspor primitif OS berbahaya seperti command execution, file write, delete recursive, dan process exit. Jika hooks/migrations dapat diubah oleh pihak tidak tepercaya, dampaknya setara remote code execution pada host.
2. **Medium**: rate limiting default nonaktif untuk instalasi baru, sehingga endpoint autentikasi dan API umum rentan brute force/abuse sampai operator mengaktifkannya.
3. **Medium**: default CORS `*` dan preflight yang merefleksikan requested headers dapat memperluas attack surface pada deployment browser/API publik.
4. **Medium**: security headers belum mencakup HSTS, Referrer-Policy, Permissions-Policy, dan CSP global untuk API/static publik; `X-XSS-Protection` sudah legacy.
5. **Medium**: Docker runtime memakai Alpine 3.19 yang sudah tua untuk tanggal audit ini; perlu update base image dan supply-chain scanning rutin.
6. **Medium**: server timeout cukup longgar dan graceful shutdown 1 detik terlalu agresif untuk workload produksi; ini dapat memicu resource exhaustion atau incomplete shutdown.
7. **Low/Medium**: endpoint SQL superuser mengembalikan raw database error dan mengizinkan non-SELECT yang tidak terklasifikasi secara ketat; ini acceptable untuk admin console tetapi perlu hardening/audit trail tambahan.

## Scope dan bukti pemeriksaan

- Validasi ulang dilakukan dengan membaca/scanning seluruh file teks repository yang dapat didekode UTF-8: **931 file**, **213.989 baris**, **10.391.851 byte**; 39 file binary/non-UTF8 dilewati sebagai aset biner.
- Fokus review manual kemudian diperdalam pada jalur security/performance berisiko: `apis/`, `core/`, `plugins/jsvm/`, `tools/filesystem/`, `Dockerfile`, `go.mod`, dan `ui/package*.json`.
- Bahasa utama: Go 1.25, modul `github.com/arief-fajri/pgbase`.
- UI: Vite-based admin UI dengan dependencies `leaflet`, `pocketbase`, `vite`, `dprint`.
- Container: multi-stage Docker build menghasilkan binary `pgbase` dan menjalankan `serve --http 0.0.0.0:8090`.
- Pemeriksaan yang dijalankan:
  - `go test ./...` gagal karena environment tidak menyediakan PostgreSQL test database di `127.0.0.1:5433`, dan beberapa test kemudian panic akibat test app nil.
  - `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` gagal karena akses ke `proxy.golang.org` ditolak.
  - `npm audit --package-lock-only --audit-level=moderate` gagal karena endpoint audit npm mengembalikan HTTP 403.

## Risk register

| ID | Area | Severity | CVSS v3.1 | Standard mapping | Temuan | Dampak | Rekomendasi prioritas |
|---|---|---:|---:|---|---|---|---|
| SEC-01 | JS hooks / plugin runtime | High | 8.8 (AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H) | OWASP A05 Security Misconfiguration, A08 Software/Data Integrity; NIST SI-10, CM-7 | `$os` binding mengekspor `exec.Command`, `os.WriteFile`, `os.RemoveAll`, `os.Exit`, dan file APIs lain ke Goja runtime. | Modifikasi hook oleh aktor internal/CI/plugin compromised dapat menjalankan command OS, menghapus data, atau eksfiltrasi secret. | Buat mode production sandbox/deny-list/allow-list untuk binding OS, nonaktifkan `$os.cmd/$os.exec` secara default, dan dokumentasikan bahwa hooks adalah trusted code. Tambahkan audit log untuk perubahan hook/migration. |
| SEC-02 | Rate limiting | Medium | 6.5 (AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:L) | OWASP API4 Unrestricted Resource Consumption, API2 Broken Auth; NIST AC-7, SC-5 | Default settings menyetel `RateLimits.Enabled: false` walau rule default tersedia. | Brute force login/OTP/password reset, scraping list endpoint, dan request flood lebih mudah pada instalasi baru. | Aktifkan rate limit default untuk instalasi baru, tambahkan migration/installer warning, dan dokumentasikan baseline production. |
| SEC-03 | CORS | Medium | 6.1 (AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N) | OWASP A05, API8 Security Misconfiguration; NIST SC-7 | `Serve` default `AllowedOrigins` menjadi `*`; jika `AllowHeaders` kosong, preflight merefleksikan `Access-Control-Request-Headers`. | Data non-credentialed public API dapat dikonsumsi dari origin mana pun; konfigurasi yang keliru dengan credential akan berisiko tinggi. | Untuk production, wajib allow-list origin eksplisit; pertimbangkan menolak wildcard saat app URL bukan localhost; set allow headers eksplisit. |
| SEC-04 | Security headers | Medium | 5.4 (AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:L/A:N) | OWASP A05; NIST SC-8, SC-23 | Middleware hanya menambahkan `X-XSS-Protection`, `X-Content-Type-Options`, `X-Frame-Options`; HSTS hanya TODO dan CSP hanya untuk UI route tertentu. | Clickjacking mitigated sebagian, tetapi browser hardening belum lengkap untuk HTTPS dan embedded/static content. | Tambahkan konfigurasi `Strict-Transport-Security` untuk HTTPS, `Referrer-Policy`, `Permissions-Policy`, dan CSP global/route-specific. Hentikan penggunaan `X-XSS-Protection` atau set `0` untuk browser modern. |
| SEC-05 | Admin SQL endpoint | Medium | 6.6 (AV:N/AC:L/PR:H/UI:N/S:U/C:H/I:H/A:H) | OWASP A01 Broken Access Control, A04 Insecure Design; NIST AC-6, AU-12 | `/api/sql` menerima query arbitrary superuser; klasifikasi write query memakai prefix list, raw DB error dikirim balik. | Superuser compromise menjadi destructive DB compromise; raw errors membantu reconnaissance. | Pertahankan superuser-only, tetapi tambahkan audit event detail, optional read-only mode, statement classifier/parser yang lebih tegas, dan redaksi raw error pada production. |
| SEC-06 | Supply chain / container | Medium | 6.3 (AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:L/A:L) | OWASP A06 Vulnerable Components, A08; NIST RA-5, SR-11 | Docker runtime `alpine:3.19` dan `postgresql16-client`; scan vulnerability tidak berhasil di environment ini. | Exposure CVE base image/dependency jika image tidak rutin di-rebuild dan discan. | Upgrade runtime ke Alpine supported terbaru, pin digest image, aktifkan SBOM dan Trivy/Grype dalam CI. |
| PERF-01 | HTTP server lifecycle | Medium | N/A | NIST SC-5, CP-10 | `ReadTimeout`/`WriteTimeout` 5 menit dan `ReadHeaderTimeout` 1 menit; shutdown context hanya 1 detik. | Long-lived slow clients dan upload besar dapat menahan goroutine lebih lama; shutdown produksi bisa memutus transaksi/request aktif. | Evaluasi timeout per route, turunkan header timeout, pakai idle timeout eksplisit, dan shutdown grace 10-30 detik configurable. |
| PERF-02 | Batch processing | Medium | N/A | OWASP API4; NIST SC-5 | Batch default mendukung sampai 50 request dan max body fallback 128 MiB, diproses dalam satu transaksi dengan goroutine dan timeout. | Memori/CPU/DB lock dapat meningkat pada batch besar, terutama multipart file upload. | Tambahkan observability per batch item, limit body per item, concurrency policy, dan circuit breaker untuk endpoint mahal. |
| PERF-03 | SQL result memory | Low | N/A | OWASP API4 | SQL endpoint membatasi max 1000 rows, tetapi data hasil tetap dikumpulkan dalam memory sebelum respons. | Superuser query dengan row/kolom besar dapat meningkatkan penggunaan memori. | Tambahkan batas byte response, streaming pagination untuk admin SQL, dan warning saat truncation. |

## Validasi mendalam per temuan

### Matriks validasi

| ID | Status validasi | Bukti kode utama yang sudah dicek | Kesimpulan setelah validasi ulang |
|---|---|---|---|
| SEC-01 | Confirmed, conditional exploitability | `plugins/jsvm/binds.go:813-836` | Binding OS memang diekspos eksplisit. Risiko bukan dari remote unauthenticated request langsung, tetapi dari supply-chain/plugin/hook write compromise atau konfigurasi deployment yang memberi akses tulis ke hooks. Severity tetap High untuk deployment yang menjalankan hooks dari volume writable. |
| SEC-02 | Confirmed | `core/settings_model.go:173-181`; `apis/middlewares_rate_limit.go` | Batch dan rate-limit rule ada, tetapi rate limit default `Enabled: false`. Ini adalah hardening gap, bukan vulnerability implementasi limiter. |
| SEC-03 | Confirmed | `apis/serve.go:61-80`; `apis/middlewares_cors.go:122-266` | Default wildcard origin benar adanya. Middleware tidak otomatis mengaktifkan credentials, sehingga risiko default adalah exposure/cross-origin readability untuk resource yang memang bisa dibaca browser, bukan credentialed CORS takeover. |
| SEC-04 | Confirmed | `apis/middlewares.go:288-300`; `apis/serve.go:91-94`; `apis/extensions.go:31-35`; `tools/filesystem/filesystem.go:469` | Header dasar tersedia, CSP ada untuk admin UI dan file response tertentu, tetapi tidak ada HSTS/referrer/permissions policy global. |
| SEC-05 | Confirmed, admin-only | `apis/sql.go:16-24`, `apis/sql.go:58-130` | SQL console guarded oleh superuser auth, tetapi tetap arbitrary SQL by design. Risk rating disesuaikan sebagai privileged abuse/blast-radius control. |
| SEC-06 | Confirmed config risk | `Dockerfile:1-14`, `go.mod`, `ui/package-lock.json` | Dockerfile memakai `alpine:3.19`; vulnerability scan online gagal karena 403 sehingga status CVE spesifik belum terverifikasi. Tetap valid sebagai supply-chain hygiene issue. |
| PERF-01 | Confirmed | `apis/serve.go:145-180`, `apis/serve.go:297-304` | Timeout server dan shutdown grace sesuai temuan. Perlu tuning production per route/reverse proxy. |
| PERF-02 | Confirmed | `apis/batch.go:95-111`, `apis/batch.go:193-215` | Batch memproses request dalam transaksi dengan fallback body 128 MiB. Endpoint disabled default, tetapi bila diaktifkan perlu limit operasional. |
| PERF-03 | Confirmed | `apis/sql.go:69-73`, `apis/sql.go:143-176` | Rows disimpan dalam slice sebelum response, dibatasi 1000 row tetapi tanpa byte cap eksplisit. Risiko terbatas pada superuser. |

### Catatan false-positive dan pembatasan audit

- Banyak match `os.RemoveAll`/`os.WriteFile` berada di test atau cleanup internal; yang diklasifikasikan High hanya ekspor primitive OS ke runtime JS karena menjadi capability untuk kode hook.
- Beberapa pemakaian `ParseUnverifiedJWT` sudah diikuti validasi token/signature pada alur terkait; item tersebut tidak dinaikkan menjadi temuan tersendiri setelah validasi ulang.
- `safeHTTPClient` untuk OAuth2/file remote sudah memitigasi SSRF loopback/private/link-local/multicast dan DNS rebinding setelah koneksi; tidak dinaikkan sebagai issue, hanya tetap direkomendasikan whitelist tambahan bila URL berasal dari input tidak tepercaya.
- Test dan vulnerability scan dependency tidak bisa memberi hasil final karena dependensi environment eksternal diblokir/tidak tersedia; laporan tidak mengklaim status bebas CVE.

## Detail temuan keamanan

### SEC-01: Binding OS berbahaya pada JavaScript runtime

**Bukti kode:** `BindOS` menambahkan `$os.exec`/`$os.cmd` ke `exec.Command`, serta file write/delete/process exit primitives.  
**Analisis PTES:** jika attacker mendapatkan akses tulis ke `pb_hooks`/`pb_migrations`, post-exploitation langsung beralih ke OS command execution.  
**CVSS:** High 8.8 dengan asumsi attacker perlu privilege rendah untuk mengubah hook/plugin; jika path hook bisa ditulis tanpa auth, naik menjadi Critical.

**Mitigasi:**
- Sediakan `jsvm.Config{DisableOSBindings: true}` atau allow-list function untuk production.
- Pisahkan runtime migration trusted dan runtime hooks dengan capability yang berbeda.
- Jalankan container dengan non-root user, read-only rootfs, seccomp/AppArmor, dan volume data minimal.
- Log dan alert setiap perubahan file hook/migration.

### SEC-02: Rate limiting default nonaktif

**Bukti kode:** default settings mengisi rules untuk auth/create/batch/API, tetapi `Enabled` diset `false`.  
**OWASP mapping:** API4:2023 Unrestricted Resource Consumption dan API2:2023 Broken Authentication.

**Mitigasi:**
- Ubah default instalasi baru menjadi enabled, minimal untuk auth-sensitive labels.
- Installer/admin UI harus menampilkan warning bila production app berjalan tanpa rate limit.
- Pertimbangkan fail-closed untuk endpoint auth jika tidak ada external WAF/rate limiter.

### SEC-03: CORS wildcard default

**Bukti kode:** `Serve` men-default-kan `AllowedOrigins` ke `*`; CORS middleware juga default `AllowOrigins: []string{"*"}` dan ketika allow headers kosong akan menyalin requested headers.  
**Dampak:** bukan bug credential theft langsung karena credentials default false, tetapi default ini tidak sesuai baseline produksi untuk API yang memproses data sensitif.

**Mitigasi:**
- Gunakan allow-list origin eksplisit berbasis `Settings.Meta.AppURL`.
- Tolak kombinasi wildcard + credentials secara hard error.
- Set `AllowHeaders` eksplisit (`Authorization`, `Content-Type`, dsb.) untuk mengurangi variasi preflight.

### SEC-04: Security headers belum lengkap

**Bukti kode:** `securityHeaders` hanya set tiga header dan HSTS masih TODO; CSP default hanya dipasang pada route UI `/_/{path...}`.  
**Mitigasi:** tambahkan header yang configurable:
- `Strict-Transport-Security: max-age=31536000; includeSubDomains` saat HTTPS.
- `Referrer-Policy: no-referrer` atau `strict-origin-when-cross-origin`.
- `Permissions-Policy` minimal deny untuk camera/microphone/geolocation kecuali dibutuhkan.
- CSP route-specific untuk public static dan admin UI.

### SEC-05: Admin SQL endpoint perlu guardrail production

**Bukti kode:** endpoint berada di group `RequireSuperuserAuth`, validasi query wajib dan panjang maksimal 5000, lalu query arbitrary dieksekusi; write query hanya dideteksi via prefix `INSERT`, `CREATE`, `UPDATE`, `DELETE`, `DROP`, `ALTER`.  
**Catatan:** karena fitur ini ditujukan untuk superuser, ini dapat diterima bila threat model mempercayai superuser sepenuhnya. Untuk standar produksi regulated environment, fitur admin destructive harus memiliki audit dan opsi disable/read-only.

**Mitigasi:**
- Tambah setting untuk disable SQL console di production.
- Log query hash, actor, IP, waktu eksekusi, rows affected, bukan full query berisi data sensitif.
- Gunakan parser SQL dialect-aware atau whitelist SELECT-only untuk read-only mode.

## Detail temuan performance/reliability

### PERF-01: Timeout server dan shutdown

**Bukti kode:** server diset `WriteTimeout`/`ReadTimeout` 5 menit, `ReadHeaderTimeout` 1 menit, dan graceful shutdown 1 detik.  
**Risiko:** header timeout 1 menit memberi ruang slowloris lebih besar dibanding banyak baseline reverse proxy; shutdown 1 detik dapat memutus request/backup/import.

**Rekomendasi:**
- Jadikan timeout configurable; baseline: `ReadHeaderTimeout` 5-10 detik, `IdleTimeout` 60-120 detik, upload route-specific timeout.
- Shutdown grace 10-30 detik, dengan readiness drain pada orchestrator.

### PERF-02: Batch endpoint resource pressure

**Bukti kode:** batch fallback `MaxBodySize` 128 MiB, timeout default 3 detik, semua item diproses dalam transaksi.  
**Risiko:** transaksi besar menahan lock dan memory; multipart batch dapat membawa file besar.

**Rekomendasi:**
- Limit per-item dan total file bytes.
- Metrics: batch size, latency per item, rollback reason, lock wait.
- Pertimbangkan batas operation type per batch dan reject nested/heavy operation.

### PERF-03: SQL response memory

**Bukti kode:** result rows ditampung dalam slice sampai 1000 row sebelum JSON response.  
**Rekomendasi:** tambahkan batas byte response dan pagination/streaming untuk hasil besar.

## Positive controls yang sudah ada

- Body limit default 32 MiB diterapkan global dan dynamic collection body limit menghitung kebutuhan file field.
- OAuth2 remote file download memakai safe HTTP client yang menolak loopback/private/link-local/multicast IP setelah connect untuk mengurangi SSRF/dns rebinding.
- TLS server minimum TLS 1.2 saat HTTPS/autocert dipakai.
- Static file handler melakukan cleaning path dan explicit traversal check.
- SQL endpoint dibatasi superuser dan query memiliki timeout 3 menit serta limit row 1000.

## Rekomendasi roadmap

### 0-30 hari

- Aktifkan rate limit default untuk instalasi baru atau tambahkan startup warning production.
- Hardening Dockerfile: update Alpine, pin digest, jalankan sebagai non-root, tambah healthcheck.
- Tambah security headers dasar dan HSTS untuk HTTPS.
- Dokumentasikan threat model JS hooks sebagai trusted code dan tambahkan opsi disable dangerous OS bindings.

### 30-60 hari

- Implement setting SQL console: disabled/read-only/full; audit event untuk setiap query.
- Tambah CI dependency scanning: `govulncheck`, `npm audit`/OSV, Trivy/Grype untuk container.
- Tambah benchmark dan load test untuk `/api/batch`, record CRUD list/filter, realtime SSE, backup/restore.

### 60-90 hari

- Capability-based sandbox untuk JS runtime.
- Per-route timeout/body limit policy dan observability (OpenTelemetry metrics/traces).
- Threat modeling formal STRIDE + PTES test cases untuk auth, file, batch, SQL, and realtime modules.

## Production checklist

- [ ] CORS allow-list eksplisit per environment.
- [ ] Rate limit enabled dan diuji untuk auth, OTP, password reset, batch, CRUD list.
- [ ] Reverse proxy mengaktifkan TLS modern, HSTS, request size limit, slowloris protection.
- [ ] Container non-root, read-only root FS bila memungkinkan, capability drop all.
- [ ] Secrets tidak disimpan di repo; encryption key, SMTP/S3 creds dari secret manager.
- [ ] Backup terenkripsi, diuji restore, dan akses backup memakai least privilege.
- [ ] Audit log untuk superuser actions, SQL console, backup/restore, settings changes.
- [ ] SBOM dan vulnerability scan di CI/CD.
