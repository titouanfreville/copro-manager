resource "google_storage_bucket" "this" {
  name     = var.name
  project  = var.project_id
  location = var.location

  uniform_bucket_level_access = true
  # Belt-and-braces: refuse any future IAM binding that would grant
  # allUsers/allAuthenticatedUsers regardless of policy. NFR12 wants this
  # bucket strictly private.
  public_access_prevention = "enforced"
  force_destroy            = false

  versioning {
    enabled = var.versioning
  }

  dynamic "lifecycle_rule" {
    for_each = var.delete_after_days > 0 ? [1] : []
    content {
      condition {
        age = var.delete_after_days
      }
      action {
        type = "Delete"
      }
    }
  }

  # NFR22: when versioning is enabled, prune noncurrent versions after
  # 30 days so deleted/overwritten objects don't accumulate forever.
  dynamic "lifecycle_rule" {
    for_each = var.versioning ? [1] : []
    content {
      condition {
        with_state                 = "ARCHIVED"
        days_since_noncurrent_time = 30
      }
      action {
        type = "Delete"
      }
    }
  }

  cors {
    origin = var.cors_origins
    method = ["GET", "PUT", "POST", "DELETE", "HEAD"]
    # GCS uses `responseHeader` for both Access-Control-Expose-Headers AND
    # the preflight Access-Control-Allow-Headers allowlist, so any custom
    # request header the browser must echo (e.g. x-goog-content-length-range
    # baked into the signed PUT URL) has to be listed here too.
    response_header = [
      "Content-Type",
      "Authorization",
      "x-goog-content-length-range",
    ]
    max_age_seconds = 3600
  }

  lifecycle {
    # Documents bucket holds receipts/contracts; refuse Terraform-side
    # destroy. force_destroy=false above is the GCP-side complement.
    prevent_destroy = true
  }
}

resource "google_storage_bucket_iam_member" "writers" {
  for_each = toset(var.writer_members)
  bucket   = google_storage_bucket.this.name
  role     = "roles/storage.objectAdmin"
  member   = each.value
}
