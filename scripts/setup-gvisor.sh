#!/usr/bin/env bash
# Install gVisor (runsc) on an Ubuntu x86_64 dev host from the official apt
# repository (https://gvisor.dev/docs/user_guide/install/), for the Phase 6
# gVisor sandbox backend (LEETFORCE_SANDBOX=gvisor). Idempotent; run as the
# normal user, it calls sudo. Nothing is piped into a shell: the signing key is
# downloaded to a file, dearmored into a keyring, and apt verifies the packages.
set -euo pipefail

KEYRING=/usr/share/keyrings/gvisor-archive-keyring.gpg
LIST=/etc/apt/sources.list.d/gvisor.list

if [ "$(uname -m)" != "x86_64" ]; then
  echo "unsupported architecture $(uname -m); LeetForce dev hosts are x86_64 (ADR 0003)" >&2
  exit 1
fi

if [ ! -f "$KEYRING" ]; then
  key=$(mktemp)
  curl -fsSL -o "$key" https://gvisor.dev/archive.key
  sudo gpg --dearmor --yes -o "$KEYRING" "$key"
  rm -f "$key"
fi
if [ ! -f "$LIST" ]; then
  echo "deb [arch=$(dpkg --print-architecture) signed-by=$KEYRING] https://storage.googleapis.com/gvisor/releases release main" |
    sudo tee "$LIST" >/dev/null
fi
sudo apt-get update -qq
sudo apt-get install -y -qq runsc
runsc --version
