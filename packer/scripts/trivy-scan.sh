#!/usr/bin/env bash
# Scan the image being built (CLAUDE.md: the Packer runner AMI build runs a Trivy scan).
# Trivy comes from the official Aqua apt repository (no piped installer) and is
# removed afterwards so it does not ship in the AMI. Fixable HIGH/CRITICAL fails the build.
# 30 minute limit: the default 5 minutes made the scan time out on a t3.small (Phase 13 log).
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

apt-get install -y -qq wget gnupg apt-transport-https
wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor >/usr/share/keyrings/trivy.gpg
echo "deb [signed-by=/usr/share/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb generic main" \
  >/etc/apt/sources.list.d/trivy.list
apt-get update -qq
apt-get install -y -qq trivy
trivy --version

rc=0
trivy rootfs --timeout 30m --scanners vuln,secret --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 \
  --skip-dirs /proc,/sys,/dev,/run,/tmp,/opt/nsjail-src \
  --skip-files '/etc/ssh/ssh_host_*_key' / || rc=$?
# The skipped host keys are deleted in cleanup.sh and regenerated at first boot.

apt-get purge -y -qq trivy
rm -f /etc/apt/sources.list.d/trivy.list /usr/share/keyrings/trivy.gpg
exit "$rc"
