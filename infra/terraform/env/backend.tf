terraform {
  backend "gcs" {
    bucket = "copro-494909-tfstate"
    prefix = "env/main"
  }
}
