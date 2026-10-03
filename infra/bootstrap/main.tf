# The one LeetForce bucket (ADR 0021). Prefixes:
#   problems/  test-data bundles (written by the API, read by runners)
#   tfstate/   Terraform state for infra/aws and infra/neon (owner only)
# It lives in its own stack so `terraform destroy` on infra/aws can never remove the
# test data or the state, and prevent_destroy makes a destroy here fail as well.

data "aws_caller_identity" "current" {}

locals {
  # Bucket names are global. The account id makes this one unique without a random
  # suffix. infra/aws derives the same name for its IAM policies.
  bucket = "leetforce-${data.aws_caller_identity.current.account_id}-data"
}

resource "aws_s3_bucket" "data" {
  bucket = local.bucket

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_public_access_block" "data" {
  bucket                  = aws_s3_bucket.data.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "data" {
  bucket = aws_s3_bucket.data.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "data" {
  bucket = aws_s3_bucket.data.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "data" {
  bucket = aws_s3_bucket.data.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Old state versions are useful for recovery but not forever. Test bundles are kept
# (a rejudge may need an old test-set version).
resource "aws_s3_bucket_lifecycle_configuration" "data" {
  bucket = aws_s3_bucket.data.id

  rule {
    id     = "expire-old-state-versions"
    status = "Enabled"
    filter {
      prefix = "tfstate/"
    }
    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }

  rule {
    id     = "abort-incomplete-uploads"
    status = "Enabled"
    filter {}
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  depends_on = [aws_s3_bucket_versioning.data]
}

data "aws_iam_policy_document" "tls_only" {
  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.data.arn, "${aws_s3_bucket.data.arn}/*"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "data" {
  bucket     = aws_s3_bucket.data.id
  policy     = data.aws_iam_policy_document.tls_only.json
  depends_on = [aws_s3_bucket_public_access_block.data]
}
