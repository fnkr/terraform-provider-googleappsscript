terraform {
  required_providers {
    googleappsscript = {
      source  = "fnkr/googleappsscript"
      version = "~> 1.0"
    }
  }
}

# Uses Application Default Credentials unless configured.
provider "googleappsscript" {}
