variable "project_id" {
  type    = string
  default = "copro-494909"
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

variable "scheduler_region" {
  description = "Region for Cloud Scheduler. europe-west9 (Paris) is not yet a supported scheduler region; europe-west1 (Belgium) is the closest available."
  type        = string
  default     = "europe-west1"
}

variable "alerts_scan_cron" {
  description = "Cron schedule for the daily alerts scan (missing-receipt cadence + seasonal balance). Defaults to 07:00 Europe/Paris — fires an hour after materialize-recurring so newly-pending rows are already in place."
  type        = string
  default     = "0 7 * * *"
}

variable "vapid_private_key" {
  description = <<-EOT
    Web Push VAPID private key. Generate locally with `webpush-go` keygen
    and pass through tfvars (sensitive — do not commit). Pair with
    vapid_public_key. Empty values disable push fan-out (the in-app
    feed still works).
  EOT
  type        = string
  default     = ""
  sensitive   = true
}

variable "vapid_public_key" {
  description = "Web Push VAPID public key. Also exposed to the SvelteKit app at build time as PUBLIC_VAPID_PUBLIC_KEY."
  type        = string
  default     = ""
}

variable "vapid_subject" {
  description = "mailto: URL the push services use to contact the app owner if delivery fails."
  type        = string
  default     = "mailto:dev@example.invalid"
}

variable "billing_account_id" {
  description = <<-EOT
    GCP billing account ID for the project. Required to provision the
    monthly billing alert (NFR27, €5/month). Find via
    `gcloud billing accounts list`. Leave empty to skip the budget — the
    project will still bill normally, you just won't get the alert.
  EOT
  type        = string
  default     = ""
}

variable "billing_alert_eur_per_month" {
  description = "Monthly spend ceiling that fires the GCP budget alert (EUR)."
  type        = number
  default     = 5
}

variable "enable_firestore_backups" {
  description = <<-EOT
    Provision a weekly Firestore export → GCS job. Adds a 3rd Cloud
    Scheduler job (Cloud Scheduler offers 3 free jobs/month/billing
    account; beyond that ≈ $0.10/job/month). GCS storage for the backups
    fits in the 5 GB free tier at this scale. Recommended ON for any
    deployment carrying real data.
  EOT
  type        = bool
  default     = false
}

variable "firestore_backup_retention_days" {
  description = "Days to retain Firestore export bundles in the backup bucket. NFR22 calls for 7."
  type        = number
  default     = 7
}
