variable "project_id" {
  type = string
}

variable "name" {
  type = string
}

variable "location" {
  type    = string
  default = "europe-west9"
}

variable "versioning" {
  type    = bool
  default = true
}

variable "delete_after_days" {
  description = "Object lifecycle: hard delete after N days (0 = never)"
  type        = number
  default     = 0
}

variable "cors_origins" {
  type    = list(string)
  default = ["*"]
}

variable "writer_members" {
  description = "IAM members granted objectAdmin (e.g. service accounts)"
  type        = list(string)
  default     = []
}
