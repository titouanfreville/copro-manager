resource "google_firestore_database" "default" {
  project          = var.project_id
  name             = "default"
  location_id      = var.location
  type             = "FIRESTORE_NATIVE"
  concurrency_mode = "OPTIMISTIC"

  app_engine_integration_mode = "DISABLED"

  # Production data lives here. Two safety nets:
  #  - GCP-side `delete_protection_state` blocks `gcloud firestore databases
  #    delete` (must explicitly disable first).
  #  - Terraform-side `prevent_destroy` makes any plan that would replace or
  #    destroy this resource fail at plan time.
  delete_protection_state = "DELETE_PROTECTION_ENABLED"

  lifecycle {
    prevent_destroy = true
  }
}
