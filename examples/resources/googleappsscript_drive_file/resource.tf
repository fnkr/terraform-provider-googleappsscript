resource "googleappsscript_drive_file" "example" {
  name = "Example"
  type = "spreadsheet"
}

resource "googleappsscript_project" "bound" {
  title     = "Example"
  parent_id = googleappsscript_drive_file.example.id
  files = {
    "appsscript.json" = jsonencode({ timeZone = "Etc/UTC", runtimeVersion = "V8" })
    "Code.gs"         = file("${path.module}/src/Code.gs")
  }
}
