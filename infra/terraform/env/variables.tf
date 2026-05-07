variable "project_id" {
  type    = string
  default = "copro-manager"
}

variable "region" {
  type    = string
  default = "europe-west9"
}

variable "firestore_location" {
  description = "Firestore is region-scoped; eur3 covers Europe with multi-region durability."
  type        = string
  default     = "eur3"
}

variable "docs_bucket_name" {
  type    = string
  default = "copro-manager-docs"
}

variable "web_domain" {
  description = "Custom domain (without scheme) for the SvelteKit PWA, if any"
  type        = string
  default     = ""
}
