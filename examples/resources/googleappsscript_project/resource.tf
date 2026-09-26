resource "googleappsscript_project" "example" {
  title = "Example"
  files = {
    "appsscript.json" = jsonencode({
      timeZone         = "Etc/UTC"
      exceptionLogging = "STACKDRIVER"
      runtimeVersion   = "V8"
      webapp           = { executeAs = "USER_DEPLOYING", access = "ANYONE_ANONYMOUS" }
    })
    "Code.gs"    = file("${path.module}/src/Code.gs")
    "Index.html" = file("${path.module}/src/Index.html")
  }
}
