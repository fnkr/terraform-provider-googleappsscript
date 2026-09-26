terraform {
  required_providers {
    googleappsscript = {
      source = "fnkr/googleappsscript"
    }
  }
}

# Uses Application Default Credentials unless configured.
provider "googleappsscript" {}
