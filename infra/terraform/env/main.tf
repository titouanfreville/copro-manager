module "artifact_registry" {
  source = "../modules/artifact-registry"

  project_id    = var.project_id
  location      = var.region
  repository_id = "api"
}

module "firestore" {
  source = "../modules/firestore"

  project_id = var.project_id
  location   = var.firestore_location
}

module "firebase_auth" {
  source = "../modules/firebase-auth"

  project_id = var.project_id
  authorized_domains = compact([
    "${var.project_id}.web.app",
    "${var.project_id}.firebaseapp.com",
    var.web_domain,
  ])
}

module "firebase_hosting" {
  source = "../modules/firebase-hosting"

  project_id = var.project_id
  site_id    = var.project_id

  depends_on = [module.firebase_auth]
}

module "docs_bucket" {
  source = "../modules/storage-bucket"

  project_id = var.project_id
  name       = var.docs_bucket_name
  location   = var.region

  cors_origins = compact([
    "http://localhost:5173",
    "https://${var.project_id}.web.app",
    var.web_domain != "" ? "https://${var.web_domain}" : "",
  ])
}

# Runtime SA for the API. Created at the env layer (rather than inside
# module.api) so we can grant secret/bucket IAM to it before the Cloud Run
# service is provisioned, avoiding a circular module dependency.
resource "google_service_account" "api_runtime" {
  project      = var.project_id
  account_id   = "api-runtime"
  display_name = "Runtime SA for api"
}

resource "google_project_iam_member" "api_runtime_roles" {
  # Least-privilege project-level grants. Storage access is bucket-scoped
  # below (api_docs_writer); never grant `roles/storage.objectAdmin` at
  # project level — it would let a compromised runtime wipe tfstate.
  # Firebase Auth: viewer is enough to verify ID tokens via the Admin SDK
  # (it pulls JWKS without needing admin write).
  # Vertex AI user: required for Gemini calls (meter OCR; document
  # classifier and chat assistant land later as additional callers).
  for_each = toset([
    "roles/aiplatform.user",
    "roles/datastore.user",
    "roles/firebaseauth.viewer",
  ])

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.api_runtime.email}"
}

# Self-impersonation: api-runtime needs iam.serviceAccounts.signBlob on
# itself to sign V4 GCS upload/download URLs (storage/setup.go calls
# iamcredentials.SignBlob with its own email resolved from the metadata
# server). GCP does not grant this implicitly.
resource "google_service_account_iam_member" "api_runtime_self_token_creator" {
  service_account_id = google_service_account.api_runtime.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.api_runtime.email}"
}

locals {
  # Production overrides rendered as a single YAML chunk and stored in
  # Secret Manager. Mounted into the API container at /etc/secrets/prod.yml
  # and merged on top of conf/main.yml via CONFIG_FILE. Keeps the file-based
  # config model AGENTS.md mandates (no `${VAR}` expansion).
  api_prod_config_yaml = yamlencode({
    middlewares = {
      # NFR14: dev-only bypass MUST stay disabled in deployed configs.
      # Pin to false explicitly so a stale conf/main.yml or env-var leak
      # cannot enable bypass auth in prod.
      allow_bypass    = false
      bypass_auth_key = ""
      admin_api_key   = var.admin_api_key
    }
    storage = {
      signing_service_account = google_service_account.api_runtime.email
    }
    push = {
      private_key = var.vapid_private_key
      public_key  = var.vapid_public_key
      subject     = var.vapid_subject
      ttl_seconds = 86400
    }
    logger = {
      env = "prod"
    }
    api = {
      cors = {
        allowed_origins = compact([
          "https://${var.project_id}.web.app",
          "https://${var.project_id}.firebaseapp.com",
          var.web_domain != "" ? "https://${var.web_domain}" : "",
        ])
      }
    }
  })
}

resource "google_secret_manager_secret" "api_prod_config" {
  project   = var.project_id
  secret_id = "api-prod-config"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "api_prod_config" {
  secret      = google_secret_manager_secret.api_prod_config.id
  secret_data = local.api_prod_config_yaml
}

resource "google_secret_manager_secret_iam_member" "api_prod_config_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.api_prod_config.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.api_runtime.email}"
}

module "api" {
  source = "../modules/cloud-run"

  project_id                    = var.project_id
  region                        = var.region
  service_name                  = "api"
  allow_unauthenticated         = true
  runtime_service_account_email = google_service_account.api_runtime.email

  env = {
    # Layered config: defaults from baked-in main.yml, prod overrides from
    # the Secret Manager YAML mounted at /etc/secrets/prod.yml.
    CONFIG_FILE = "/app/conf/main.yml:/etc/secrets/prod.yml"
  }

  secret_files = [{
    secret_id = google_secret_manager_secret.api_prod_config.secret_id
    mount_dir = "/etc/secrets"
    file_name = "prod.yml"
  }]

  depends_on = [
    module.artifact_registry,
    module.firestore,
    module.firebase_auth,
    module.docs_bucket,
    google_secret_manager_secret_version.api_prod_config,
    google_secret_manager_secret_iam_member.api_prod_config_accessor,
  ]
}

# Grant the API runtime SA write access to the docs bucket
resource "google_storage_bucket_iam_member" "api_docs_writer" {
  bucket = module.docs_bucket.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${module.api.runtime_service_account_email}"
}

# Daily Cloud Scheduler job that materializes recurring expense templates.
# Created only when admin_api_key is set — empty key would mean the API
# rejects every call (admin disabled), so there's no point firing the
# scheduler. The lazy on-load path on /expenses keeps working in either
# case.
resource "google_cloud_scheduler_job" "materialize_recurring" {
  count = var.admin_api_key == "" ? 0 : 1

  name             = "materialize-recurring"
  project          = var.project_id
  region           = var.scheduler_region
  description      = "Daily run of the recurring expense template materializer"
  schedule         = var.scheduler_cron
  time_zone        = "Europe/Paris"
  attempt_deadline = "60s"

  retry_config {
    retry_count          = 2
    min_backoff_duration = "30s"
    max_backoff_duration = "300s"
  }

  http_target {
    http_method = "POST"
    uri         = "${module.api.service_url}/admin/expense-templates/materialize-recurring"
    headers = {
      "Authorization" = "AdminKey ${var.admin_api_key}"
      "Content-Type"  = "application/json"
    }
  }
}

# Daily Cloud Scheduler job for the alerts time-based scan: ages out
# missing-receipt alerts (D+3, D+10, every 15 days thereafter) and fires
# the seasonal balance nudge on Jul 15 + Dec 15. Idempotent. Same
# AdminKey gate as materialize-recurring.
resource "google_cloud_scheduler_job" "alerts_scan" {
  count = var.admin_api_key == "" ? 0 : 1

  name             = "alerts-scan"
  project          = var.project_id
  region           = var.scheduler_region
  description      = "Daily run of the alerts time-based scan (missing-receipt + seasonal)"
  schedule         = var.alerts_scan_cron
  time_zone        = "Europe/Paris"
  attempt_deadline = "60s"

  retry_config {
    retry_count          = 2
    min_backoff_duration = "30s"
    max_backoff_duration = "300s"
  }

  http_target {
    http_method = "POST"
    uri         = "${module.api.service_url}/admin/alerts/scan"
    headers = {
      "Authorization" = "AdminKey ${var.admin_api_key}"
      "Content-Type"  = "application/json"
    }
  }
}

# ───────────────────────────────────────────────────────────────────────
# Firestore backups (NFR22) — weekly export to GCS via Cloud Scheduler.
# Free-tier-friendly: bucket sized for two-foyer dataset (likely <50MB
# per export, well inside 5GB free tier); only one extra Cloud Scheduler
# job (3rd of the project — still inside the 3-free-jobs/month quota at
# Google's current pricing).
# ───────────────────────────────────────────────────────────────────────

resource "google_storage_bucket" "firestore_backups" {
  count = var.enable_firestore_backups ? 1 : 0

  name     = "${var.project_id}-firestore-backups"
  project  = var.project_id
  location = var.region

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false

  lifecycle_rule {
    condition {
      age = var.firestore_backup_retention_days
    }
    action {
      type = "Delete"
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_service_account" "firestore_backup" {
  count = var.enable_firestore_backups ? 1 : 0

  project      = var.project_id
  account_id   = "firestore-backup"
  display_name = "Service account for the weekly Firestore export job"
}

# Datastore importExportAdmin lets the SA call Firestore Admin's
# exportDocuments RPC. Project-scoped because that's the smallest scope
# the role accepts.
resource "google_project_iam_member" "firestore_backup_export" {
  count = var.enable_firestore_backups ? 1 : 0

  project = var.project_id
  role    = "roles/datastore.importExportAdmin"
  member  = "serviceAccount:${google_service_account.firestore_backup[0].email}"
}

resource "google_storage_bucket_iam_member" "firestore_backup_writer" {
  count = var.enable_firestore_backups ? 1 : 0

  bucket = google_storage_bucket.firestore_backups[0].name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.firestore_backup[0].email}"
}

resource "google_cloud_scheduler_job" "firestore_export" {
  count = var.enable_firestore_backups ? 1 : 0

  name             = "firestore-export"
  project          = var.project_id
  region           = var.scheduler_region
  description      = "Weekly Firestore export to GCS (NFR22)"
  schedule         = "0 4 * * 0" # Sunday 04:00 Paris
  time_zone        = "Europe/Paris"
  attempt_deadline = "180s"

  retry_config {
    retry_count          = 1
    min_backoff_duration = "60s"
    max_backoff_duration = "300s"
  }

  http_target {
    http_method = "POST"
    uri         = "https://firestore.googleapis.com/v1/projects/${var.project_id}/databases/default:exportDocuments"
    headers = {
      "Content-Type" = "application/json"
    }
    # Body is a JSON-encoded string. yamlencode + jsonencode would
    # over-escape; jsonencode + base64encode is what Cloud Scheduler's
    # http_target.body expects (it base64-decodes before sending).
    body = base64encode(jsonencode({
      outputUriPrefix = "gs://${google_storage_bucket.firestore_backups[0].name}/scheduled"
    }))

    oidc_token {
      service_account_email = google_service_account.firestore_backup[0].email
      audience              = "https://firestore.googleapis.com/"
    }
  }
}

# ───────────────────────────────────────────────────────────────────────
# Billing budget alert (NFR27).
# ───────────────────────────────────────────────────────────────────────

resource "google_billing_budget" "monthly_cap" {
  count = var.billing_account_id == "" ? 0 : 1

  billing_account = var.billing_account_id
  display_name    = "${var.project_id} monthly cap"

  budget_filter {
    projects = ["projects/${var.project_id}"]
  }

  amount {
    specified_amount {
      currency_code = "EUR"
      units         = tostring(var.billing_alert_eur_per_month)
    }
  }

  threshold_rules {
    threshold_percent = 0.5
  }
  threshold_rules {
    threshold_percent = 0.9
  }
  threshold_rules {
    threshold_percent = 1.0
  }
}
