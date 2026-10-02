#!/usr/bin/env bash
# Provision an Ubuntu 24.04 x86_64 dev host for LeetForce Phase 1 (see ADR 0003).
# Idempotent: safe to run repeatedly. Run as the normal user; it calls sudo.
set -euo pipefail

SWAP_FILE=/swapfile
SWAP_SIZE=2G
NSJAIL_DIR="$HOME/nsjail"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-latest}"

if [ "$(uname -m)" != "x86_64" ]; then
  echo "unsupported architecture $(uname -m); LeetForce dev hosts are x86_64 (ADR 0003)" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -qq
sudo apt-get install -y -qq build-essential git curl make pkg-config nodejs npm \
  autoconf bison flex libtool libprotobuf-dev libnl-route-3-dev protobuf-compiler \
  openjdk-21-jdk-headless redis-server

if ! swapon --show | grep -q "$SWAP_FILE"; then
  sudo fallocate -l "$SWAP_SIZE" "$SWAP_FILE"
  sudo chmod 600 "$SWAP_FILE"
  sudo mkswap "$SWAP_FILE" >/dev/null
  sudo swapon "$SWAP_FILE"
fi
grep -q "$SWAP_FILE" /etc/fstab || echo "$SWAP_FILE none swap sw 0 0" | sudo tee -a /etc/fstab >/dev/null

if ! command -v go >/dev/null 2>&1 && [ ! -x /usr/local/go/bin/go ]; then
  go_file=$(curl -fsSL "https://go.dev/dl/?mode=json" | grep -o 'go[0-9.]*linux-amd64.tar.gz' | head -1)
  curl -fsSL -o /tmp/go.tgz "https://go.dev/dl/$go_file"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf /tmp/go.tgz
fi
grep -q /usr/local/go/bin "$HOME/.bashrc" ||
  echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >>"$HOME/.bashrc"
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

if ! command -v nsjail >/dev/null 2>&1; then
  [ -d "$NSJAIL_DIR" ] || git clone -q https://github.com/google/nsjail "$NSJAIL_DIR"
  make -C "$NSJAIL_DIR" -j"$(nproc)"
  sudo cp "$NSJAIL_DIR/nsjail" /usr/local/bin/nsjail
fi

if ! command -v golangci-lint >/dev/null 2>&1; then
  go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
fi

if ! command -v goose >/dev/null 2>&1; then
  go install github.com/pressly/goose/v3/cmd/goose@latest
fi

if ! command -v trivy >/dev/null 2>&1; then
  # Official Aqua Security apt repository (key is dearmored into a keyring, no piped installer).
  sudo apt-get install -y -qq wget gnupg lsb-release apt-transport-https
  wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key |
    gpg --dearmor | sudo tee /usr/share/keyrings/trivy.gpg >/dev/null
  echo "deb [signed-by=/usr/share/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb generic main" |
    sudo tee /etc/apt/sources.list.d/trivy.list >/dev/null
  sudo apt-get update -qq
  sudo apt-get install -y -qq trivy
fi

echo "--- environment ---"
go version
command -v nsjail
golangci-lint --version
trivy --version | head -1
echo "cgroup fs: $(stat -fc %T /sys/fs/cgroup) (want cgroup2fs)"
echo "controllers: $(cat /sys/fs/cgroup/cgroup.controllers) (want cpu memory pids)"
sudo -n true && echo "passwordless sudo: ok"
