# Terraform Provider for Google Apps Script

Manage [Google Apps Script](https://developers.google.com/apps-script) projects, versions and deployments with Terraform, including scripts bound to Google Docs, Sheets, Forms and Slides.

```hcl
terraform {
  required_providers {
    googleappsscript = {
      source  = "fnkr/googleappsscript"
      version = "~> 0.1"
    }
  }
}

provider "googleappsscript" {}

resource "googleappsscript_project" "example" {
  title = "Example"
  files = {
    "appsscript.json" = jsonencode({ timeZone = "Etc/UTC", runtimeVersion = "V8" })
    "Code.gs"         = file("${path.module}/src/Code.gs")
  }
}

resource "googleappsscript_version" "example" {
  script_id = googleappsscript_project.example.id

  lifecycle {
    replace_triggered_by = [googleappsscript_project.example]
  }
}

resource "googleappsscript_deployment" "example" {
  script_id      = googleappsscript_project.example.id
  version_number = googleappsscript_version.example.version_number
}
```

See the [documentation](https://registry.terraform.io/providers/fnkr/googleappsscript/latest/docs) for authentication and all resources.

## Pitfalls

- The Apps Script API must be turned on for the user at [script.google.com/home/usersettings](https://script.google.com/home/usersettings), otherwise every request fails with 403.
- The Apps Script API does not support service accounts. Use user credentials that include the `drive` scope.
- Destroying a standalone project moves it to the Drive trash. Container-bound projects and versions cannot be deleted via the API and are only removed from state; bound projects are trashed along with their container.

## Development

Requires [Go](https://go.dev) and [Terraform](https://developer.hashicorp.com/terraform/install).

```sh
make build     # build
make test      # tests against an in-memory fake of the Google APIs
make testacc   # same tests against the real APIs (creates real resources)
make generate  # regenerate docs
```

To use a local build, add a [dev override](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers) for `fnkr/googleappsscript` pointing at your `GOBIN`.

`make testacc` uses Application Default Credentials, `GOOGLE_CREDENTIALS` or `GOOGLE_OAUTH_ACCESS_TOKEN`; see the [provider docs](docs/index.md#authentication) for setup.

Releases are built and signed by GoReleaser when a `v*` tag is pushed; the Terraform Registry picks up the GitHub release.

## License

[MIT](LICENSE)
