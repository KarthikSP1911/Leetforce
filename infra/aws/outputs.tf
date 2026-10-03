output "control_public_ip" {
  value = aws_instance.control.public_ip
}

# Known after the group has launched its instances; run `terraform refresh` (or
# apply again) if this is empty right after the first apply.
output "runner_public_ips" {
  value = data.aws_instances.runner.public_ips
}

output "runner_autoscaling_group" {
  value = aws_autoscaling_group.runner.name
}

output "runner_security_group_id" {
  value = aws_security_group.runner.id
}
