resource "google_storage_bucket" "this" {
  name     = var.name
  project  = var.project_id
  location = var.location

  uniform_bucket_level_access = true
  force_destroy               = false

  versioning {
    enabled = var.versioning
  }

  lifecycle_rule {
    condition {
      age = var.delete_after_days
    }
    action {
      type = "Delete"
    }
  }

  cors {
    origin          = var.cors_origins
    method          = ["GET", "PUT", "POST", "DELETE", "HEAD"]
    response_header = ["Content-Type", "Authorization"]
    max_age_seconds = 3600
  }
}

resource "google_storage_bucket_iam_member" "writers" {
  for_each = toset(var.writer_members)
  bucket   = google_storage_bucket.this.name
  role     = "roles/storage.objectAdmin"
  member   = each.value
}
