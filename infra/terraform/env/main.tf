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

locals {
  # Deterministic SA email — the cloud-run module hardcodes
  # account_id = "${var.service_name}-runtime", so we can pre-compute the
  # email without creating a circular `module.api` self-reference.
  api_runtime_sa_email = "api-runtime@${var.project_id}.iam.gserviceaccount.com"
}

module "api" {
  source = "../modules/cloud-run"

  project_id            = var.project_id
  region                = var.region
  service_name          = "api"
  allow_unauthenticated = true

  env = {
    GCP_PROJECT                     = var.project_id
    DOCS_BUCKET                     = module.docs_bucket.name
    STORAGE_SIGNING_SERVICE_ACCOUNT = local.api_runtime_sa_email
    LOG_ENV                         = "prod"
    WEB_DOMAIN                      = var.web_domain != "" ? var.web_domain : "${var.project_id}.web.app"
    ALLOW_BYPASS                    = "false"
  }

  depends_on = [
    module.artifact_registry,
    module.firestore,
    module.firebase_auth,
    module.docs_bucket,
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
  region           = var.region
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
