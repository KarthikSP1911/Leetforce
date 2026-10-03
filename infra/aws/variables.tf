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
