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

module "api" {
  source = "../modules/cloud-run"

  project_id            = var.project_id
  region                = var.region
  service_name          = "api"
  allow_unauthenticated = true

  env = {
    GCP_PROJECT  = var.project_id
    DOCS_BUCKET  = module.docs_bucket.name
    LOG_ENV      = "prod"
    WEB_DOMAIN   = var.web_domain != "" ? var.web_domain : "${var.project_id}.web.app"
    ALLOW_BYPASS = "false"
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
