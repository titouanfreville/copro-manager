terraform {
  backend "gcs" {
    bucket = "copro-manager-tfstate"
    prefix = "env/main"
  }
}
