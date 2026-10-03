#!/usr/bin/env bash
# Leave nothing host-specific or build-only in the image.
set -euo pipefail
rm -rf /opt/nsjail-src # source tree; the binary stays in /usr/local/bin/nsjail
apt-get autoremove -y -qq
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
rm -f /etc/ssh/ssh_host_* # cloud-init regenerates them, so every instance gets its own
cloud-init clean --logs
truncate -s 0 /etc/machine-id
rm -f /home/ubuntu/.ssh/authorized_keys # EC2 injects the launch key pair again on boot
