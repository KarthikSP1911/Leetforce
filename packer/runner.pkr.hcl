# LeetForce runner AMI: Ubuntu 24.04 x86_64 + hardening + toolchains + pinned nsjail
# + the unprivileged runner service (installed but NOT enabled: Phase 13 supplies
# /etc/leetforce/runner.env at boot and enables it).
#
#   make build-runner-linux          # bin/runner-linux-amd64
#   make packer-validate             # free, no credentials
#   make build-ami                   # BILLABLE: launches a t3.small builder for ~15 minutes
packer {
  required_version = ">= 1.10"
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1.3"
    }
    ansible = {
      source  = "github.com/hashicorp/ansible"
      version = "~> 1.1"
    }
  }
}

variable "region" {
  type    = string
  default = "ap-south-1"
}

variable "instance_type" {
  type    = string
  default = "t3.small"
}

variable "runner_binary" {
  type        = string
  default     = "../bin/runner-linux-amd64"
  description = "Cross-compiled runner, built by make build-runner-linux."
}

locals {
  timestamp = formatdate("YYYYMMDD-hhmmss", timestamp())
}

source "amazon-ebs" "runner" {
  region        = var.region
  instance_type = var.instance_type
  ssh_username  = "ubuntu"
  ami_name      = "leetforce-runner-${local.timestamp}"

  # Default VPC and a default-VPC subnet: no custom networking (cost rules).
  associate_public_ip_address = true

  source_ami_filter {
    filters = {
      name                = "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    owners      = ["099720109477"] # Canonical
    most_recent = true
  }

  # IMDSv2 only, on the builder and on every instance launched from the AMI.
  imds_support = "v2.0"
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  encrypt_boot = true
  launch_block_device_mappings {
    device_name           = "/dev/sda1"
    volume_size           = 15
    volume_type           = "gp3"
    delete_on_termination = true
  }

  tags = {
    Project   = "leetforce"
    ManagedBy = "packer"
    Name      = "leetforce-runner-${local.timestamp}"
  }
}

build {
  name    = "runner"
  sources = ["source.amazon-ebs.runner"]

  provisioner "shell" {
    inline = [
      "cloud-init status --wait",
      "sudo apt-get update -qq",
      # The base image is weeks old: apply pending security fixes (the Trivy gate failed on openssl)
      # and refresh its preinstalled snaps (snapd, core22, amazon-ssm-agent) to the latest revisions.
      "sudo DEBIAN_FRONTEND=noninteractive apt-get upgrade -y -qq",
      "sudo snap refresh",
      "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ansible",
    ]
  }

  # Runs on the builder itself; localhost is placed in the "runners" group so
  # both the hardening and the runner_host roles apply.
  provisioner "ansible-local" {
    playbook_file    = "../ansible/site.yml"
    role_paths       = ["../ansible/roles/hardening", "../ansible/roles/runner_host"]
    inventory_groups = ["runners"]
  }

  provisioner "file" {
    source      = var.runner_binary
    destination = "/tmp/runner"
  }

  provisioner "file" {
    source      = "../scripts/runner"
    destination = "/tmp/runner-install"
  }

  provisioner "shell" {
    inline = [
      "chmod +x /tmp/runner-install/install-runner.sh",
      "sudo /tmp/runner-install/install-runner.sh /tmp/runner",
      "sudo systemctl disable leetforce-runner || true",
      "rm -rf /tmp/runner /tmp/runner-install",
    ]
  }

  # Fails the build on fixable HIGH/CRITICAL findings in the finished image.
  provisioner "shell" {
    script          = "scripts/trivy-scan.sh"
    execute_command = "sudo -E bash '{{ .Path }}'"
  }

  provisioner "shell" {
    script          = "scripts/cleanup.sh"
    execute_command = "sudo -E bash '{{ .Path }}'"
  }

  post-processor "manifest" {
    output = "packer-manifest.json"
  }
}
