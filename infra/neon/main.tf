# The Neon project already exists and holds the data, so this stack adopts it
# with an import block instead of creating a new one (ADR 0020). The
# prevent_destroy guard makes `terraform destroy` here fail loudly.
import {
  to = neon_project.leetforce
  id = var.project_id
}

resource "neon_project" "leetforce" {
  name                      = var.project_name
  region_id                 = var.region_id
  pg_version                = var.pg_version
  history_retention_seconds = var.history_retention_seconds

  lifecycle {
    prevent_destroy = true
    # Branches, roles, databases and compute settings are owned by the Neon
    # console and goose migrations, not by this stack.
    ignore_changes = [branch]
  }
}
