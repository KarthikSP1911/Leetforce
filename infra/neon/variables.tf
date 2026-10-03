variable "project_id" {
  description = "Id of the existing Neon project to adopt (Neon console > Project settings > General)."
  type        = string
}

variable "project_name" {
  description = "Project name as shown in the Neon console."
  type        = string
  default     = "leetforce"
}

variable "region_id" {
  description = "Neon region of the existing project (the pooler host name shows it: aws-ap-southeast-1)."
  type        = string
  default     = "aws-ap-southeast-1"
}

variable "pg_version" {
  description = "Postgres major version of the existing project; must match the console or plan proposes a change."
  type        = number
  default     = 17
}

variable "history_retention_seconds" {
  description = "Point-in-time restore window. 21600 (6 h) is the free-plan maximum; raise it on a paid plan."
  type        = number
  default     = 21600
}
