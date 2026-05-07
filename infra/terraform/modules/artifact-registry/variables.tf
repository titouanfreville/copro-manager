variable "project_id" {
  type = string
}

variable "location" {
  type    = string
  default = "europe-west9"
}

variable "repository_id" {
  type    = string
  default = "api"
}

variable "description" {
  type    = string
  default = "Container images for copro-manager"
}
