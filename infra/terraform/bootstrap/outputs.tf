output "state_bucket" {
  description = "GCS bucket holding the terraform state for the env stack"
  value       = google_storage_bucket.tfstate.name
}

output "wif_provider" {
  description = "Set as GitHub secret WIF_PROVIDER"
  value       = "projects/${data.google_project.this.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github.workload_identity_pool_id}/providers/${google_iam_workload_identity_pool_provider.github.workload_identity_pool_provider_id}"
}

output "wif_service_account" {
  description = "Set as GitHub secret WIF_SERVICE_ACCOUNT"
  value       = google_service_account.deploy.email
}
