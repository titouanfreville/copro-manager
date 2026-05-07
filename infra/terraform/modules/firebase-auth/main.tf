resource "google_firebase_project" "default" {
  provider = google-beta
  project  = var.project_id
}

resource "google_identity_platform_config" "default" {
  provider = google-beta
  project  = var.project_id

  sign_in {
    allow_duplicate_emails = false

    email {
      enabled           = true
      password_required = true
    }
  }

  authorized_domains = concat(
    ["localhost"],
    var.authorized_domains,
  )

  depends_on = [google_firebase_project.default]
}
