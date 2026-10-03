output "control_public_ip" {
  value = aws_instance.control.public_ip
}

output "runner_public_ips" {
  value = aws_instance.runner[*].public_ip
}

output "runner_security_group_id" {
  value = aws_security_group.runner.id
}
