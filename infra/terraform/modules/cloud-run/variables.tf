variable "project_id" {
  type = string
}

variable "region" {
  type    = string
  default = "europe-west9"
}

variable "service_name" {
  type    = string
  default = "api"
}

variable "image" {
  description = "Initial container image (overwritten by deploy workflow afterwards)"
  type        = string
  default     = "us-docker.pkg.dev/cloudrun/container/hello"
}

variable "cpu" {
  type    = string
  default = "1"
}

variable "memory" {
  type    = string
  default = "512Mi"
}

variable "max_instances" {
  type    = number
  default = 3
}

variable "env" {
  description = "Plain env vars for the service (no secrets)"
  type        = map(string)
  default     = {}
}

variable "allow_unauthenticated" {
  description = "Allow public (unauthenticated) HTTP invocations"
  type        = bool
  default     = true
}

variable "runtime_roles" {
  description = "IAM roles granted to the Cloud Run runtime SA"
  type        = list(string)
  default = [
    "roles/datastore.user",
    "roles/firebase.sdkAdminServiceAgent",
    "roles/firebaseauth.admin",
    "roles/storage.objectAdmin",
  ]
}
