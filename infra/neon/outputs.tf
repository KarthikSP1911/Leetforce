output "project_id" {
  value = neon_project.leetforce.id
}

# Sensitive: read with `terraform output -raw database_url`. Phase 13 stores it
# in SSM Parameter Store; it must never reach a runner (CLAUDE.md security rules).
output "database_url" {
  value     = neon_project.leetforce.connection_uri
  sensitive = true
}
