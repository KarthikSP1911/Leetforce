# Phase 13 log: Cloud deployment

Running log. Secrets, key contents and public IPs are never written here; names only.

## File and path index
| Path | What |
|---|---|
| `infra/aws/terraform.tfvars` | git-ignored; `owner_cidr` (owner's public IP /32) and `key_name` |
| `C:\Users\karth\.aws\{config,credentials}` | outside the repo; profile `leetforce` |

## Session 1

### Decisions (owner answers in chat, 2026-10-03)
- B: S3 backend with locking (small recurring cost, to go in the README cost table). Owner answered.
- C: owner did not understand the question; Claude's default applies: runners stay as systemd services on AMI-based hosts, outside k3s (to be an ADR).
- D: owner said "whichever is convenient"; Claude's default: GitHub Actions OIDC role.
- E: owner OK with `up`, test, `down` in the same session (typed confirmation each time).
- Phase 12 recap question: not answered.

### Steps
1. Owner: created IAM user `leetforce-terraform` (AdministratorAccess, to be narrowed in Phase 16) and an access key; ran `aws configure --profile leetforce` (region `ap-south-1`). Verified with `aws sts get-caller-identity --profile leetforce`: account matches the owner's, user `leetforce-terraform`.
2. Claude: read-only `aws ec2 describe-key-pairs` (key pairs `leetforce`, `ec2-learning-pk`) and `describe-instances` (only `leetforce-dev`, t3.micro, running).
3. Claude: `git checkout -b phase/13-cloud-deployment`, `git tag phase-13-start`.
4. Claude: wrote git-ignored `infra/aws/terraform.tfvars` (owner IP, key `leetforce`). Mistake avoided: `terraform.tfvars.example` names key `leetforce-dev`, which does not exist in the account.
5. Claude: `AWS_PROFILE=leetforce terraform -chdir=infra/aws init` then `plan`: **25 to add, 0 to change, 0 to destroy**; nothing created. (Phase 12 exit criterion for `infra/aws` plan now met.)

### Open
- `infra/neon` plan: needs `NEON_API_KEY` and the Neon project id (`.env` has only `DATABASE_URL`).
- AMI build: waiting for owner's confirmation (billable).

### Packer AMI build (owner confirmed "okay build", billable)
6. Claude: `make` is not installed on Windows, so `make build-ami` could not run (attempt 1, exit 127, nothing created). Ran its two steps by hand: `go build` with `GOOS=linux GOARCH=amd64` in `runner/` (output `bin/runner-linux-amd64`, git-ignored), then Packer in Docker with credentials exported from the `leetforce` profile as `AWS_*` environment variables (never printed).
7. Attempt 2 failed in seconds: `packer init` and `packer build` ran in two separate `--rm` containers, so the plugins were gone ("Missing plugins"). Nothing created. Fix: one container, `sh -c 'packer init . && packer build -color=false .'`.
8. Attempt 3 running: builder `t3.small` launched, SSH connected, Ansible hardening role passed on a real host for the first time (no failed task), runner_host role installed toolchains and built nsjail, Trivy scan stage reached. Log: `%TEMP%\build-ami2.log`. Result and AMI id: see below once finished.

### Unit `feat/13-s3-storage` (owner: "s3 everywhere, keep rustfs comment, single s3")
9. Owner decision: real S3 in every environment, RustFS kept as a commented-out block, one bucket. Claude's reading: one bucket with prefixes `problems/` and `tfstate/`, in its own stack so `arena.sh down` cannot remove it (ADR 0021).
10. `storage/storage.go`: endpoint set and both keys empty now uses the EC2 instance role (`credentials.NewIAM`); only one key set is an error. `storage/storage_test.go` updated. `go vet` and `go test ./storage` pass on Windows (the S3 round-trip test skips without an endpoint). `gofmt -l` lists the files on Windows only because of CRLF working-tree endings; `make fmt lint` and the wider `make test` still have to run on the dev host.
11. `infra/bootstrap/` (new stack, local state): bucket `leetforce-<account id>-data`, versioning, SSE-S3, public access block, TLS-only policy, 90-day expiry of old `tfstate/` versions, `prevent_destroy`. `terraform validate` ok; read-only `terraform plan`: 7 to add, 0 to change, 0 to destroy. **Not applied.**
12. `infra/aws/main.tf`: runner role gets `GetObject` on `problems/*`; control role adds `PutObject`; neither can read `tfstate/`. `validate` ok.
13. `docker-compose.yml`: `s3` service and `s3-data` volume commented out with instructions; `.env.example` rewritten for real S3 (keys empty on AWS hosts); Makefile comments and `tf-validate` (now three stacks) updated. Mistake noted: `scripts/test-destroy-isolation.sh` still assumes two stacks and a local backend; it must be revisited when state moves to S3.
14. `scripts/scan-staged.sh` (Trivy secret scan on the dev host): clean. Commits `0bac658` (storage) and `34ff87a` (infra).

### Owner action noted
- The owner pasted the `DATABASE_URL` (with the Neon password) into chat. Claude advised rotating the password in the Neon console and updating `.env` and the dev host; not yet confirmed done.

### Open (additions)
- Apply `infra/bootstrap` (needs typed confirmation), upload bundles, switch the `infra/aws` and `infra/neon` backends to S3 (`use_lockfile`), re-point `test-destroy-isolation.sh`.
- Cross-region latency: Neon is in ap-southeast-1, AWS in ap-south-1; keep and measure in Phase 16.
