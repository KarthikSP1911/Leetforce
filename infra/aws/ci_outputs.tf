output "github_deploy_role_arn" {
  description = "Set this as the GitHub repository variable AWS_DEPLOY_ROLE_ARN."
  value       = aws_iam_role.github_deploy.arn
}
