package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestProject(t *testing.T) {
	title := acctest.RandomWithPrefix("tf-acc")
	runTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig(title, "Code.gs", "function a() {}"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("googleappsscript_project.test", "id"),
					resource.TestCheckResourceAttr("googleappsscript_project.test", "title", title),
					resource.TestCheckResourceAttr("googleappsscript_project.test", "files.%", "2"),
				),
			},
			{
				ResourceName:      "googleappsscript_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccProjectConfig(title+"-renamed", "lib/util.gs", "function b() {}"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("googleappsscript_project.test", "title", title+"-renamed"),
					resource.TestCheckResourceAttr("googleappsscript_project.test", "files.lib/util.gs", "function b() {}"),
					resource.TestCheckNoResourceAttr("googleappsscript_project.test", "files.Code.gs"),
				),
			},
		},
	})
}

func TestProject_validation(t *testing.T) {
	config := `
resource "googleappsscript_project" "test" {
  title = "test"
  files = %s
}
`
	runTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(config, `{ "appsscript.json" = "{}", "Code.js" = "" }`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`unsupported file "Code.js"`),
			},
			{
				Config:      fmt.Sprintf(config, `{ "Code.gs" = "" }`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`files must contain appsscript.json`),
			},
		},
	})
}

func testAccProjectConfig(title, file, source string) string {
	return fmt.Sprintf(`
resource "googleappsscript_project" "test" {
  title = %q
  files = {
    "appsscript.json" = jsonencode({
      timeZone         = "Etc/UTC"
      exceptionLogging = "STACKDRIVER"
      runtimeVersion   = "V8"
      webapp           = { executeAs = "USER_DEPLOYING", access = "MYSELF" }
    })
    %q = %q
  }
}
`, title, file, source)
}
