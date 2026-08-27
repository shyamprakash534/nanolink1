variable "aws_region" {
  type        = string
  default     = "us-east-1"
  description = "AWS deployment region"
}

variable "environment" {
  type        = string
  default     = "production"
  description = "Deployment environment"
}

variable "app_name" {
  type        = string
  default     = "nanolink"
  description = "Application name"
}

variable "db_password" {
  type        = string
  default     = "SuperSecretSecurePass123!"
  sensitive   = true
  description = "RDS PostgreSQL master password"
}
