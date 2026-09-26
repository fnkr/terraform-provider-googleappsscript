package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestDeployment(t *testing.T) {
	title := acctest.RandomWithPrefix("tf-acc")
	runTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentConfig(title, "function doGet() { return ContentService.createTextOutput('v1') }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("googleappsscript_version.test", "version_number", "1"),
					resource.TestCheckResourceAttr("googleappsscript_deployment.test", "version_number", "1"),
					resource.TestMatchResourceAttr("googleappsscript_deployment.test", "web_app_url", regexp.MustCompile(`^https://script.google.com/`)),
				),
			},
			{
				ResourceName:      "googleappsscript_deployment.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["googleappsscript_deployment.test"]
					return rs.Primary.Attributes["script_id"] + "/" + rs.Primary.ID, nil
				},
			},
			{
				Config: testAccDeploymentConfig(title, "function doGet() { return ContentService.createTextOutput('v2') }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("googleappsscript_version.test", "version_number", "2"),
					resource.TestCheckResourceAttr("googleappsscript_deployment.test", "version_number", "2"),
				),
			},
		},
	})
}

func testAccDeploymentConfig(title, source string) string {
	return testAccProjectConfig(title, "Code.gs", source) + fmt.Sprintf(`
resource "googleappsscript_version" "test" {
  script_id   = googleappsscript_project.test.id
  description = %q

  lifecycle {
    replace_triggered_by = [googleappsscript_project.test]
  }
}

resource "googleappsscript_deployment" "test" {
  script_id      = googleappsscript_project.test.id
  version_number = googleappsscript_version.test.version_number
  description    = "test"
}
`, source)
}
