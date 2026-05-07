variable "project_id" {
  description = "GCP project ID for copro-manager"
  type        = string
  default     = "copro-manager"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "europe-west9"
}

variable "github_owner" {
  description = "GitHub user/org that owns the repo"
  type        = string
  default     = "titouanfreville"
}

variable "github_repo" {
  description = "GitHub repository (owner/repo)"
  type        = string
  default     = "titouanfreville/copro-manager"
}
