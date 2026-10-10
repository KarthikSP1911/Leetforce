variable "region" {
  description = "AWS region; the dev host already lives in ap-south-1."
  type        = string
  default     = "ap-south-1"
}

variable "owner_cidr" {
  description = "Your public IP as a /32 (for example 203.0.113.7/32). The only source allowed to reach SSH and the k3s API."
  type        = string

  validation {
    condition     = can(cidrhost(var.owner_cidr, 0)) && endswith(var.owner_cidr, "/32")
    error_message = "owner_cidr must be a single address in /32 form."
  }
}

variable "key_name" {
  description = "Name of an existing EC2 key pair for SSH."
  type        = string
}

variable "control_instance_type" {
  description = "k3s server (API, web, observability agents). x86 (ADR 0003)."
  type        = string
  default     = "t3.small"
}

variable "runner_instance_type" {
  description = "Runner hosts (nsjail needs a real Linux host, not a container)."
  type        = string
  default     = "t3.small"
}

variable "runner_count" {
  description = "Number of runner instances. 0 keeps only the control host."
  type        = number
  default     = 1
}

variable "runner_ami_id" {
  description = "Packer-built runner AMI (packer/runner.pkr.hcl). Empty falls back to stock Ubuntu 24.04, which Ansible then hardens."
  type        = string
  default     = ""
}

variable "root_volume_gib" {
  type    = number
  default = 15
}

variable "ssm_prefix" {
  description = "SSM Parameter Store path prefix (Phase 13 fills it)."
  type        = string
  default     = "/leetforce"
}

# ---- k3s agent nodes for KEDA-scaled runner pods (Phase 17, ADR 0029; runner-k3s.tf) ----

variable "runner_node_count" {
  description = "Number of k3s agent EC2 nodes that host the KEDA-scaled runner pods. FIXED: KEDA scales pods, not nodes, and pods beyond the capacity of these nodes stay Pending. 0 (the default) creates nothing from runner-k3s.tf. Independent of runner_count (the standalone runner hosts)."
  type        = number
  default     = 0

  validation {
    condition     = var.runner_node_count >= 0 && var.runner_node_count <= 10 && floor(var.runner_node_count) == var.runner_node_count
    error_message = "runner_node_count must be a whole number from 0 to 10."
  }
}

variable "runner_node_instance_type" {
  description = "Instance type of the k3s agent nodes. A runner pod requests 768 Mi of memory, so a t3.small (2 GiB) fits one pod; raise it to fit more pods per node."
  type        = string
  default     = "t3.small"
}

variable "runner_node_ami_id" {
  description = "AMI of the agent nodes. Empty uses stock Ubuntu 24.04 (the user data then installs k3s and the AppArmor profile at first boot); the Ansible hardening role is NOT applied to such nodes."
  type        = string
  default     = ""
}

variable "runner_node_k3s_version" {
  description = "k3s version of the agents. Keep equal to k3s_server_version in ansible/roles/k3s_server/defaults/main.yml (scripts/check-nsjail-pin.sh compares them)."
  type        = string
  default     = "v1.37.1+k3s1"
}

variable "runner_node_volume_gib" {
  description = "Root volume of an agent node. Holds the runner image (about 1.5 GiB), job directories and the problem cache."
  type        = number
  default     = 20
}
