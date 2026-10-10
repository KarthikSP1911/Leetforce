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

# ---- Phase 17 (ADR 0029): k3s agent nodes. Empty / null while runner_node_count = 0. ----

output "control_private_ip" {
  description = "Private address of the control host: the k3s agents join https://<this>:6443."
  value       = aws_instance.control.private_ip
}

output "k3s_agent_autoscaling_group" {
  value = one(aws_autoscaling_group.k3s_agent[*].name)
}

output "k3s_agent_security_group_id" {
  value = one(aws_security_group.k3s_agent[*].id)
}
