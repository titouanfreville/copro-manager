resource "google_firebase_hosting_site" "this" {
  provider = google-beta
  project  = var.project_id
  site_id  = var.site_id

  lifecycle {
    # `app_id` is auto-populated when a Firebase web app is registered
    # against the site (via console or `firebase apps:create`). Don't
    # let TF nullify it on subsequent applies.
    ignore_changes = [app_id]
  }
}
