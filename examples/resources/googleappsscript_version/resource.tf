resource "googleappsscript_version" "example" {
  script_id = googleappsscript_project.example.id

  # Create a new version whenever the project changes.
  lifecycle {
    replace_triggered_by = [googleappsscript_project.example]
  }
}
