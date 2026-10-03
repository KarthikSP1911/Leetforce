# Default VPC only: no custom VPC, endpoints or NAT gateway (CLAUDE.md cost rules).
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_caller_identity" "current" {}

# Stock Ubuntu 24.04 LTS x86_64, resolved through Canonical's public SSM parameter.
data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}

locals {
  subnet_id     = sort(data.aws_subnets.default.ids)[0]
  runner_ami_id = var.runner_ami_id != "" ? var.runner_ami_id : data.aws_ssm_parameter.ubuntu.value
}

# ---- Security groups: nothing is open to the world ----

resource "aws_security_group" "control" {
  name_prefix = "leetforce-control-"
  description = "LeetForce k3s control host: SSH and k3s API from the owner only"
  vpc_id      = data.aws_vpc.default.id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "runner" {
  name_prefix = "leetforce-runner-"
  description = "LeetForce runners: SSH from the owner only, no other inbound"
  vpc_id      = data.aws_vpc.default.id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "control_ssh" {
  security_group_id = aws_security_group.control.id
  description       = "SSH from owner"
  cidr_ipv4         = var.owner_cidr
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

resource "aws_vpc_security_group_ingress_rule" "control_k3s_api" {
  security_group_id = aws_security_group.control.id
  description       = "k3s API from owner"
  cidr_ipv4         = var.owner_cidr
  ip_protocol       = "tcp"
  from_port         = 6443
  to_port           = 6443
}

resource "aws_vpc_security_group_ingress_rule" "control_from_runners" {
  security_group_id            = aws_security_group.control.id
  description                  = "k3s API for agents on runner hosts"
  referenced_security_group_id = aws_security_group.runner.id
  ip_protocol                  = "tcp"
  from_port                    = 6443
  to_port                      = 6443
}

# The API is served by k3s Traefik on port 80. Owner for testing; runners report results here
# over the private network (a source-group rule, so nothing else can reach it).
resource "aws_vpc_security_group_ingress_rule" "control_http_owner" {
  security_group_id = aws_security_group.control.id
  description       = "API ingress from owner"
  cidr_ipv4         = var.owner_cidr
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
}

resource "aws_vpc_security_group_ingress_rule" "control_http_runners" {
  security_group_id            = aws_security_group.control.id
  description                  = "API ingress from runner hosts"
  referenced_security_group_id = aws_security_group.runner.id
  ip_protocol                  = "tcp"
  from_port                    = 80
  to_port                      = 80
}

resource "aws_vpc_security_group_ingress_rule" "runner_ssh" {
  security_group_id = aws_security_group.runner.id
  description       = "SSH from owner"
  cidr_ipv4         = var.owner_cidr
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

# Metrics ports 9101/9102 stay on localhost (Phase 11); Prometheus reaches them
# through an SSH tunnel, so there is deliberately no rule for them.

locals {
  # Egress by port, not "all": web and package mirrors, Upstash Redis (TLS on
  # 6379), DNS and NTP. The control host also needs Postgres for Neon (5432).
  # Destination IPs of Upstash, Neon and S3 are not fixed, so the CIDR stays open.
  egress_common = {
    https = { protocol = "tcp", port = 443 }
    http  = { protocol = "tcp", port = 80 }
    redis = { protocol = "tcp", port = 6379 }
    dns   = { protocol = "udp", port = 53 }
    ntp   = { protocol = "udp", port = 123 }
  }
}

resource "aws_vpc_security_group_egress_rule" "control" {
  for_each = merge(local.egress_common, { postgres = { protocol = "tcp", port = 5432 } })

  security_group_id = aws_security_group.control.id
  description       = "Outbound ${each.key}"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = each.value.protocol
  from_port         = each.value.port
  to_port           = each.value.port
}

resource "aws_vpc_security_group_egress_rule" "runner" {
  for_each = local.egress_common

  security_group_id = aws_security_group.runner.id
  description       = "Outbound ${each.key}"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = each.value.protocol
  from_port         = each.value.port
  to_port           = each.value.port
}

# ---- IAM: runners read only /leetforce/runner/*, never the database URL ----

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "runner_ssm" {
  statement {
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = ["arn:aws:ssm:${var.region}:${data.aws_caller_identity.current.account_id}:parameter${var.ssm_prefix}/runner/*"]
  }
}

data "aws_iam_policy_document" "control_ssm" {
  statement {
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = ["arn:aws:ssm:${var.region}:${data.aws_caller_identity.current.account_id}:parameter${var.ssm_prefix}/*"]
  }
}

# The single bucket from infra/bootstrap (ADR 0021). Runners may only read test bundles;
# the control host (API) may also write them. Neither can see tfstate/.
locals {
  data_bucket_arn = "arn:aws:s3:::leetforce-${data.aws_caller_identity.current.account_id}-data"
}

data "aws_iam_policy_document" "runner_s3" {
  statement {
    actions   = ["s3:GetObject"]
    resources = ["${local.data_bucket_arn}/problems/*"]
  }
  statement {
    actions   = ["s3:ListBucket"]
    resources = [local.data_bucket_arn]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["problems/*"]
    }
  }
}

data "aws_iam_policy_document" "control_s3" {
  statement {
    actions   = ["s3:GetObject", "s3:PutObject"]
    resources = ["${local.data_bucket_arn}/problems/*"]
  }
  # HeadBucket (the API's startup check) sends no prefix, so a prefix condition would deny it.
  # Listing shows key names only; reading tfstate/ objects stays denied.
  statement {
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [local.data_bucket_arn]
  }
}

resource "aws_iam_role" "runner" {
  name_prefix        = "leetforce-runner-"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

resource "aws_iam_role_policy" "runner_ssm" {
  name   = "read-runner-parameters"
  role   = aws_iam_role.runner.id
  policy = data.aws_iam_policy_document.runner_ssm.json
}

resource "aws_iam_role_policy" "runner_s3" {
  name   = "read-problem-bundles"
  role   = aws_iam_role.runner.id
  policy = data.aws_iam_policy_document.runner_s3.json
}

resource "aws_iam_instance_profile" "runner" {
  name_prefix = "leetforce-runner-"
  role        = aws_iam_role.runner.name
}

resource "aws_iam_role" "control" {
  name_prefix        = "leetforce-control-"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

resource "aws_iam_role_policy" "control_ssm" {
  name   = "read-leetforce-parameters"
  role   = aws_iam_role.control.id
  policy = data.aws_iam_policy_document.control_ssm.json
}

resource "aws_iam_role_policy" "control_s3" {
  name   = "readwrite-problem-bundles"
  role   = aws_iam_role.control.id
  policy = data.aws_iam_policy_document.control_s3.json
}

resource "aws_iam_instance_profile" "control" {
  name_prefix = "leetforce-control-"
  role        = aws_iam_role.control.name
}

# ---- Instances ----

resource "aws_instance" "control" {
  ami                         = data.aws_ssm_parameter.ubuntu.value
  instance_type               = var.control_instance_type
  subnet_id                   = local.subnet_id
  vpc_security_group_ids      = [aws_security_group.control.id]
  key_name                    = var.key_name
  iam_instance_profile        = aws_iam_instance_profile.control.name
  associate_public_ip_address = true # no NAT gateway, so this is the only way out (billed, see README)

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required" # IMDSv2 only
    http_put_response_hop_limit = 2          # pods (the API) reach the instance role: one extra network hop. Runner hosts stay at 1.
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = var.root_volume_gib
    encrypted   = true
  }

  tags = {
    Name = "leetforce-control"
    Role = "control"
  }

  lifecycle {
    ignore_changes = [ami] # a newer stock AMI must not replace a running host
  }
}

# Runner instances are an Auto Scaling group: see runner-asg.tf.
