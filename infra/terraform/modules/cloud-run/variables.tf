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

variable "runtime_service_account_email" {
  description = "Email of the runtime SA used by the Cloud Run service. Created externally so the caller can grant secret/bucket IAM without circular module dependencies."
  type        = string
}

# Secret Manager secrets mounted as files into the container.
# IAM (`roles/secretmanager.secretAccessor`) must be granted by the caller —
# the module stays neutral so the env stack can audit secret access.
variable "secret_files" {
  description = "Secret Manager secrets to mount as files in the container"
  type = list(object({
    secret_id = string # Secret name in Secret Manager (project-scoped)
    mount_dir = string # Directory inside the container (e.g. /etc/secrets)
    file_name = string # File name inside mount_dir (e.g. prod.yml)
  }))
  default = []
}
