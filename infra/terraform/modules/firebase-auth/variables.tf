variable "project_id" {
  type = string
}

variable "authorized_domains" {
  description = "Domains authorized to sign users in via Firebase Auth"
  type        = list(string)
  default     = []
}
