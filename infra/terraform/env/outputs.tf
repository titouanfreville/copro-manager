output "api_url" {
  value = module.api.service_url
}

output "artifact_registry_url" {
  value = module.artifact_registry.repository_url
}

output "docs_bucket" {
  value = module.docs_bucket.name
}

output "firebase_hosting_url" {
  value = module.firebase_hosting.default_url
}

output "api_runtime_service_account" {
  value = module.api.runtime_service_account_email
}
