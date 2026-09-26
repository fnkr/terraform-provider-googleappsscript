package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestDriveFile(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")
	runTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testDriveFileConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("googleappsscript_drive_file.test", "name", name),
					resource.TestCheckResourceAttr("googleappsscript_drive_file.test", "type", "spreadsheet"),
					resource.TestMatchResourceAttr("googleappsscript_drive_file.test", "url", regexp.MustCompile(`^https://`)),
					resource.TestCheckResourceAttrPair("googleappsscript_project.test", "parent_id", "googleappsscript_drive_file.test", "id"),
				),
			},
			{
				ResourceName:      "googleappsscript_drive_file.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testDriveFileConfig(name + "-renamed"),
				Check:  resource.TestCheckResourceAttr("googleappsscript_drive_file.test", "name", name+"-renamed"),
			},
		},
	})
}

func testDriveFileConfig(name string) string {
	return fmt.Sprintf(`
resource "googleappsscript_drive_file" "test" {
  name = %[1]q
  type = "spreadsheet"
}

resource "googleappsscript_project" "test" {
  title     = %[1]q
  parent_id = googleappsscript_drive_file.test.id
  files = {
    "appsscript.json" = jsonencode({ timeZone = "Etc/UTC", runtimeVersion = "V8" })
    "Code.gs"         = "function onOpen() {}"
  }
}
`, name)
}
