terraform {
  required_providers {
    googleappsscript = {
      source  = "fnkr/googleappsscript"
      version = "~> 0.1"
    }
  }
}

# Uses Application Default Credentials unless configured.
provider "googleappsscript" {}
