resource "googleappsscript_deployment" "example" {
  script_id      = googleappsscript_project.example.id
  version_number = googleappsscript_version.example.version_number
  description    = "Production"
}

output "url" {
  value = googleappsscript_deployment.example.web_app_url
}
