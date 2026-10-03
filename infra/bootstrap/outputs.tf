output "bucket" {
  description = "Name for LEETFORCE_S3_BUCKET and for the S3 backend `bucket` setting."
  value       = aws_s3_bucket.data.bucket
}
