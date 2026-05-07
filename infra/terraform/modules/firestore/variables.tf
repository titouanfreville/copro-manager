variable "project_id" {
  type = string
}

variable "location" {
  description = "Firestore location (multi-region or region). europe-west9 is supported."
  type        = string
  default     = "eur3"
}
