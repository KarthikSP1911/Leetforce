# k3s agent nodes for the KEDA-scaled runner pods (Phase 17, ADR 0029).
# WRITTEN, NEVER VALIDATED, PLANNED OR APPLIED: no terraform command has been run on this file.
#
# What this is: a launch template and an Auto Scaling group of EC2 hosts that join the existing
# k3s control host as agents, carrying the label leetforce.dev/pool=runner and the taint
# leetforce.dev/runner=true:NoSchedule so only runner pods (k8s/charts/leetforce-runner) land on
# them. The node count is FIXED by runner_node_count (desired = min = max). KEDA changes the
# number of PODS; when the pods want more room than the nodes have, they stay Pending until the
# owner raises runner_node_count. Node autoscaling is out of scope.
#
# runner_node_count = 0 (the default) creates NOTHING from this file: every resource below has
# count = 0, including the IAM role and the security group.
#
# The standalone runner hosts in runner-asg.tf are a separate path (runner_count). Both can run
# at once; they are consumers of the same Redis consumer group.

locals {
  k3s_agents_enabled = var.runner_node_count > 0
  k3s_agent_ami_id   = var.runner_node_ami_id != "" ? var.runner_node_ami_id : data.aws_ssm_parameter.ubuntu.value
  # Written by scripts/k3s/push-agent-token.sh. Inside /leetforce/runner/ like the runner settings
  # (the owner's decision), but the policy below allows reading this one parameter only.
  k3s_agent_token_param = "${var.ssm_prefix}/runner/K3S_AGENT_TOKEN"
}

# ---- Security group: the k3s agent ports to the control host only ----

resource "aws_security_group" "k3s_agent" {
  count = local.k3s_agents_enabled ? 1 : 0

  name_prefix = "leetforce-k3s-agent-"
  description = "LeetForce k3s agent nodes: SSH from the owner, cluster ports from the control host and each other"
  vpc_id      = data.aws_vpc.default.id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "k3s_agent_ssh" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id = aws_security_group.k3s_agent[0].id
  description       = "SSH from owner"
  cidr_ipv4         = var.owner_cidr
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

# Flannel VXLAN carries pod traffic between nodes (the default k3s backend), UDP 8472.
resource "aws_vpc_security_group_ingress_rule" "k3s_agent_vxlan_from_control" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.k3s_agent[0].id
  description                  = "Flannel VXLAN from the control host"
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "udp"
  from_port                    = 8472
  to_port                      = 8472
}

resource "aws_vpc_security_group_ingress_rule" "k3s_agent_vxlan_from_agents" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.k3s_agent[0].id
  description                  = "Flannel VXLAN between agent nodes"
  referenced_security_group_id = aws_security_group.k3s_agent[0].id
  ip_protocol                  = "udp"
  from_port                    = 8472
  to_port                      = 8472
}

# The kubelet API (logs, exec, metrics) is called by the API server on the control host.
resource "aws_vpc_security_group_ingress_rule" "k3s_agent_kubelet_from_control" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.k3s_agent[0].id
  description                  = "Kubelet from the control host"
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "tcp"
  from_port                    = 10250
  to_port                      = 10250
}

# Control host side: the agents join and talk to the API server on 6443 and send VXLAN.
resource "aws_vpc_security_group_ingress_rule" "control_k3s_from_agents" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.control.id
  description                  = "k3s supervisor and API for agent nodes"
  referenced_security_group_id = aws_security_group.k3s_agent[0].id
  ip_protocol                  = "tcp"
  from_port                    = 6443
  to_port                      = 6443
}

resource "aws_vpc_security_group_ingress_rule" "control_vxlan_from_agents" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.control.id
  description                  = "Flannel VXLAN from agent nodes"
  referenced_security_group_id = aws_security_group.k3s_agent[0].id
  ip_protocol                  = "udp"
  from_port                    = 8472
  to_port                      = 8472
}

# The control host's egress is by port (main.tf), so the cluster ports toward the agents are added.
resource "aws_vpc_security_group_egress_rule" "control_vxlan_to_agents" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.control.id
  description                  = "Flannel VXLAN to agent nodes"
  referenced_security_group_id = aws_security_group.k3s_agent[0].id
  ip_protocol                  = "udp"
  from_port                    = 8472
  to_port                      = 8472
}

resource "aws_vpc_security_group_egress_rule" "control_kubelet_to_agents" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.control.id
  description                  = "Kubelet API on agent nodes"
  referenced_security_group_id = aws_security_group.k3s_agent[0].id
  ip_protocol                  = "tcp"
  from_port                    = 10250
  to_port                      = 10250
}

# Agent egress. Public destinations by port as for the standalone runners (HTTPS for S3, GHCR and
# the k3s installer, HTTP for package mirrors, Redis TLS, DNS, NTP); the CIDR stays open because
# Upstash and S3 have no fixed ranges and there is no NAT or endpoint (cost rule). The cluster
# ports below go to the control host and the other agents only.
resource "aws_vpc_security_group_egress_rule" "k3s_agent" {
  # A for expression, not a conditional: the two branches of a conditional must have the same type.
  for_each = { for name, rule in local.egress_common : name => rule if local.k3s_agents_enabled }

  security_group_id = aws_security_group.k3s_agent[0].id
  description       = "Outbound ${each.key}"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = each.value.protocol
  from_port         = each.value.port
  to_port           = each.value.port
}

resource "aws_vpc_security_group_egress_rule" "k3s_agent_api_to_control" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.k3s_agent[0].id
  description                  = "k3s supervisor and API on the control host"
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "tcp"
  from_port                    = 6443
  to_port                      = 6443
}

resource "aws_vpc_security_group_egress_rule" "k3s_agent_vxlan_to_control" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.k3s_agent[0].id
  description                  = "Flannel VXLAN to the control host"
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "udp"
  from_port                    = 8472
  to_port                      = 8472
}

resource "aws_vpc_security_group_egress_rule" "k3s_agent_vxlan_to_agents" {
  count = local.k3s_agents_enabled ? 1 : 0

  security_group_id            = aws_security_group.k3s_agent[0].id
  description                  = "Flannel VXLAN between agent nodes"
  referenced_security_group_id = aws_security_group.k3s_agent[0].id
  ip_protocol                  = "udp"
  from_port                    = 8472
  to_port                      = 8472
}

# ---- IAM: S3 read of the test bundles, and the one SSM parameter that holds the join token ----

# Narrower than the standalone runners (which may read all of /leetforce/runner/*): the pods get
# their Redis URL from a Kubernetes Secret, not from SSM, so the node only needs the token.
data "aws_iam_policy_document" "k3s_agent_ssm" {
  statement {
    actions   = ["ssm:GetParameter"]
    resources = ["arn:aws:ssm:${var.region}:${data.aws_caller_identity.current.account_id}:parameter${local.k3s_agent_token_param}"]
  }
}

resource "aws_iam_role" "k3s_agent" {
  count = local.k3s_agents_enabled ? 1 : 0

  name_prefix        = "leetforce-k3s-agent-"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

resource "aws_iam_role_policy" "k3s_agent_ssm" {
  count = local.k3s_agents_enabled ? 1 : 0

  name   = "read-k3s-agent-token"
  role   = aws_iam_role.k3s_agent[0].id
  policy = data.aws_iam_policy_document.k3s_agent_ssm.json
}

# Same policy as the standalone runners: GetObject on problems/ and ListBucket for that prefix.
# Pods use it through the instance metadata service (storage.Open falls back to the host IAM role
# when no keys are set), so a compromise of a runner pod can read the hidden tests. It could
# already: the pod needs them to judge.
resource "aws_iam_role_policy" "k3s_agent_s3" {
  count = local.k3s_agents_enabled ? 1 : 0

  name   = "read-problem-bundles"
  role   = aws_iam_role.k3s_agent[0].id
  policy = data.aws_iam_policy_document.runner_s3.json
}

resource "aws_iam_instance_profile" "k3s_agent" {
  count = local.k3s_agents_enabled ? 1 : 0

  name_prefix = "leetforce-k3s-agent-"
  role        = aws_iam_role.k3s_agent[0].name
}

# ---- Launch template and Auto Scaling group ----

resource "aws_launch_template" "k3s_agent" {
  count = local.k3s_agents_enabled ? 1 : 0

  name_prefix   = "leetforce-k3s-agent-"
  image_id      = local.k3s_agent_ami_id
  instance_type = var.runner_node_instance_type
  key_name      = var.key_name

  iam_instance_profile {
    name = aws_iam_instance_profile.k3s_agent[0].name
  }

  user_data = base64encode(templatefile("${path.module}/k3s-agent-userdata.sh.tftpl", {
    region           = var.region
    token_param      = local.k3s_agent_token_param
    k3s_url          = "https://${aws_instance.control.private_ip}:6443"
    k3s_version      = var.runner_node_k3s_version
    node_label       = "leetforce.dev/pool=runner"
    node_taint       = "leetforce.dev/runner=true:NoSchedule"
    apparmor_profile = file("${path.module}/../../ansible/roles/k3s_agent/files/leetforce-runner-pod")
  }))

  network_interfaces {
    associate_public_ip_address = true # no NAT gateway (README cost table)
    security_groups             = [aws_security_group.k3s_agent[0].id]
    delete_on_termination       = true
  }

  # IMDS hop limit 2, unlike the standalone runners (1). Pods reach the instance metadata service
  # through one extra network hop (the pod network bridge), and the runner pod needs it for the
  # node role's temporary S3 credentials, so a hop limit of 1 would break S3 for every pod.
  # IMDSv2 stays required. What limits the exposure instead:
  #   - the judged program has NO network at all (its own network namespace, no interface), so
  #     it cannot reach IMDS, with or without this setting;
  #   - the pod NetworkPolicy blocks all of 169.254.0.0/16 except 169.254.169.254:80;
  #   - the role is read-only on problems/ and one SSM parameter (the join token).
  # The residual risk: a sandbox ESCAPE into the runner pod (or the node) can reach the role.
  # That is the same blast radius as an escape on today's standalone runner hosts, whose role
  # can also be reached from the host. ADR 0029 records it.
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  block_device_mappings {
    device_name = "/dev/sda1"
    ebs {
      volume_type           = "gp3"
      volume_size           = var.runner_node_volume_gib
      encrypted             = true
      delete_on_termination = true
    }
  }

  # default_tags do not reach instances launched by an Auto Scaling group.
  tag_specifications {
    resource_type = "instance"
    tags = {
      Name      = "leetforce-k3s-agent"
      Role      = "k3s-agent"
      Project   = "leetforce"
      ManagedBy = "terraform"
      Stack     = "infra-aws"
    }
  }

  tag_specifications {
    resource_type = "volume"
    tags = {
      Name      = "leetforce-k3s-agent"
      Project   = "leetforce"
      ManagedBy = "terraform"
      Stack     = "infra-aws"
    }
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_autoscaling_group" "k3s_agent" {
  count = local.k3s_agents_enabled ? 1 : 0

  name_prefix         = "leetforce-k3s-agent-"
  min_size            = var.runner_node_count
  max_size            = var.runner_node_count
  desired_capacity    = var.runner_node_count
  vpc_zone_identifier = sort(data.aws_subnets.default.ids)

  # EC2 health only: a stopped or impaired instance is replaced. A node that is up but not Ready
  # in Kubernetes is not detected here (no load balancer, no node-health hook).
  health_check_type         = "EC2"
  health_check_grace_period = 300

  launch_template {
    id      = aws_launch_template.k3s_agent[0].id
    version = aws_launch_template.k3s_agent[0].latest_version
  }

  # There is deliberately no instance_refresh block. A refresh would terminate nodes without
  # draining them, killing runner pods mid-job (the job would be reclaimed after about 90 s, but
  # that is a delay users see). A changed launch template applies to NEW instances only; to roll
  # the nodes, terminate them one at a time after `kubectl drain` (docs/phases/phase-17-log.md).
  # A replaced node leaves a stale Node object in the cluster: delete it with kubectl.

  tag {
    key                 = "Name"
    value               = "leetforce-k3s-agent"
    propagate_at_launch = false # the launch template already tags instances
  }

  tag {
    key                 = "Role"
    value               = "k3s-agent"
    propagate_at_launch = false
  }
}
