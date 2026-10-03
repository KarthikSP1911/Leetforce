# infra/neon

Terraform for the LeetForce Neon Postgres project. Separate state from `infra/aws`, so destroying the cloud stack cannot reach the database.

```bash
export NEON_API_KEY=...            # Neon console > Account > API keys; never commit it
cp terraform.tfvars.example terraform.tfvars   # set project_id
terraform init
terraform plan                     # first plan shows "1 to import"; apply only after review
```

- The project is adopted by an `import` block; `prevent_destroy` blocks `terraform destroy`.
- Local state (`terraform.tfstate`, git-ignored). Back it up: losing it only means re-importing.
- Schema is owned by goose migrations (`make migrate-up`), not Terraform.
