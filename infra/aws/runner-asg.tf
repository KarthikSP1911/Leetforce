# Runner fleet: a launch template and an Auto Scaling group pinned at runner_count
# (desired = min = max). The group only replaces failed or terminated instances; it
# does not scale on load. Each instance configures itself on first boot from SSM
# (runner-userdata.sh.tftpl). Runners are standalone systemd services, not k3s agents.

resource "aws_launch_template" "runner" {
  name_prefix   = "leetforce-runner-"
  image_id      = local.runner_ami_id
  instance_type = var.runner_instance_type
  key_name      = var.key_name

  iam_instance_profile {
    name = aws_iam_instance_profile.runner.name
  }

  user_data = base64encode(templatefile("${path.module}/runner-userdata.sh.tftpl", {
    ssm_prefix = var.ssm_prefix
    region     = var.region
  }))

  network_interfaces {
    associate_public_ip_address = true # no NAT gateway (README cost table)
    security_groups             = [aws_security_group.runner.id]
    delete_on_termination       = true
  }

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required" # IMDSv2 only
    http_put_response_hop_limit = 1          # blocks sandboxed code reaching IMDS through a container hop
  }

  block_device_mappings {
    device_name = "/dev/sda1"
    ebs {
      volume_type           = "gp3"
      volume_size           = var.root_volume_gib
      encrypted             = true
      delete_on_termination = true
    }
  }

  # default_tags do not reach instances launched by an Auto Scaling group.
  tag_specifications {
    resource_type = "instance"
    tags = {
      Name      = "leetforce-runner"
      Role      = "runner"
      Project   = "leetforce"
      ManagedBy = "terraform"
      Stack     = "infra-aws"
    }
  }

  tag_specifications {
    resource_type = "volume"
    tags = {
      Name      = "leetforce-runner"
      Project   = "leetforce"
      ManagedBy = "terraform"
      Stack     = "infra-aws"
    }
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_autoscaling_group" "runner" {
  name_prefix         = "leetforce-runner-"
  min_size            = var.runner_count
  max_size            = var.runner_count
  desired_capacity    = var.runner_count
  vpc_zone_identifier = sort(data.aws_subnets.default.ids)

  # EC2 health only: a stopped, terminated or impaired instance is replaced. A runner
  # whose service fails is not detected here (no load balancer); see the README.
  health_check_type         = "EC2"
  health_check_grace_period = 300

  launch_template {
    id      = aws_launch_template.runner.id
    version = aws_launch_template.runner.latest_version
  }

  # A new AMI or user data rolls the fleet. 0% healthy lets a one-runner fleet be
  # replaced (it is briefly empty; queued jobs wait in Redis).
  instance_refresh {
    strategy = "Rolling"
    preferences {
      min_healthy_percentage = 0
    }
  }

  tag {
    key                 = "Name"
    value               = "leetforce-runner"
    propagate_at_launch = false # the launch template already tags instances
  }
}

# Public addresses of the running runners (for SSH and the Prometheus tunnel).
data "aws_instances" "runner" {
  instance_state_names = ["running"]

  filter {
    name   = "tag:aws:autoscaling:groupName"
    values = [aws_autoscaling_group.runner.name]
  }
}
