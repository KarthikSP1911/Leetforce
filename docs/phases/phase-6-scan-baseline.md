# Phase 6 security scan baseline

Run on 2026-10-02 on the dev host, in a clean clone of `main` (commit c6ea98e, identical to the tip of `phase/6-sandbox-hardening`). Nothing was changed; this is a report only.

## Tool versions

- Trivy 0.75.0
- Vulnerability DB version 2, updated 2026-10-02 12:48 UTC (downloaded 2026-10-02 15:59 UTC)
- Checks bundle digest sha256:1583562f8b90ed2a071b99f0e5ffff6b57e4ceb6ca3e4796577b4e6a339eb74c

## .trivyignore

No `.trivyignore` exists in the repository, so there are no suppressions and none can be expired.

## Results

| Command | Result |
|---|---|
| `trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL .` | 0 vulnerabilities, 0 secrets, 0 misconfigurations. Scanned api, judge, queue, runner, storage go.mod and web/package-lock.json (dev dependencies suppressed by default). |
| `trivy config --severity HIGH,CRITICAL infra/` | No supported config files detected (0 findings). `infra/aws` and `infra/neon` contain no Terraform that Trivy recognised at this commit. |
| `trivy config --severity HIGH,CRITICAL k8s/` | No supported config files detected (0 findings). |
| `trivy config --severity HIGH,CRITICAL docker-compose.yml` | No supported config files detected (0 findings). Compose files are not scanned as Dockerfile/IaC by this Trivy version; images are scanned below. |
| `trivy image --severity HIGH,CRITICAL redis:7-alpine` | 4 HIGH, 0 CRITICAL (alpine 3.21.8) |
| `trivy image --severity HIGH,CRITICAL rustfs/rustfs:1.0.0` | 0 findings (alpine 3.24.1) |

Totals: 0 CRITICAL, 4 HIGH (2 distinct CVEs, each in 2 packages), 0 secrets.

## HIGH and CRITICAL findings

All in image `redis:7-alpine` (docker-compose.yml, `redis` service, local dev only).

| ID | Package | Installed | Fixed | Title |
|---|---|---|---|---|
| CVE-2026-75804 | libcrypto3 | 3.3.7-r1 | 3.3.7-r2 | OpenSSL: DoS via unenforced QUIC connection flow control |
| CVE-2026-75804 | libssl3 | 3.3.7-r1 | 3.3.7-r2 | same |
| CVE-2026-84782 | libcrypto3 | 3.3.7-r1 | 3.3.7-r2 | OpenSSL: information disclosure via DTLS handshake retransmission |
| CVE-2026-84782 | libssl3 | 3.3.7-r1 | 3.3.7-r2 | same |

Suggested fix: pull the current `redis:7-alpine` (the base image should pick up openssl 3.3.7-r2 when rebuilt upstream), or pin a newer tag/digest and rescan. If the upstream image is not yet rebuilt before the Phase 6 end scan, record it as an accepted finding in the phase log (local dev Redis only, not exposed, QUIC and DTLS are not used by Redis) or add a dated `.trivyignore` entry with an expiry. Production Redis is Upstash, so this image is not deployed.

## Notes for the end-of-phase scan

- Phase 6 may add Terraform, k8s or Packer files; re-run `trivy config` on them, since the baseline has nothing to compare against there.
- Image scan results drift daily with the vulnerability DB; rerun and compare against this table.
