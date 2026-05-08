variable "project_id" {
  type    = string
  default = "copro-manager"
}

variable "region" {
  type    = string
  default = "europe-west9"
}

variable "firestore_location" {
  description = "Firestore is region-scoped; eur3 covers Europe with multi-region durability."
  type        = string
  default     = "eur3"
}

variable "docs_bucket_name" {
  type    = string
  default = "copro-manager-docs"
}

variable "web_domain" {
  description = "Custom domain (without scheme) for the SvelteKit PWA, if any"
  type        = string
  default     = ""
}

variable "admin_api_key" {
  description = <<-EOT
    Shared secret for the /admin/* endpoints. Cloud Scheduler attaches this
    in the Authorization header when calling the daily materialization
    endpoint. Must match middlewares.admin_api_key on the server side
    (configured via the Cloud Run config-file mechanism, see AGENTS.md).
    Leave empty to disable scheduled materialization (the lazy on-load
    path on /expenses still works). Stored sensitive — do not commit
    terraform.tfvars with a non-empty value.
  EOT
  type        = string
  default     = ""
  sensitive   = true
}

variable "scheduler_cron" {
  description = "Cron schedule for the daily materialization job. Defaults to 06:00 Europe/Paris."
  type        = string
  default     = "0 6 * * *"
}
