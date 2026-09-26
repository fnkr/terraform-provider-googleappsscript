package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"google.golang.org/api/option"
)

// runTest runs tc against an in-memory fake API, or against the real API
// (using the same credentials as the provider) if TF_ACC is set.
func runTest(t *testing.T, tc resource.TestCase) {
	t.Helper()
	ctx := context.Background()

	var (
		httpClient *http.Client
		auth       option.ClientOption
	)
	if os.Getenv("TF_ACC") == "" {
		httpClient = &http.Client{Transport: newFakeAPI()}
		auth = option.WithHTTPClient(httpClient)
		prev := pollInterval
		pollInterval = 0
		t.Cleanup(func() { pollInterval = prev })
	} else {
		var err error
		if auth, err = clientOption(ctx, os.Getenv(envAccessToken), os.Getenv(envCredentials)); err != nil {
			t.Fatal(err)
		}
	}
	c, err := newClient(ctx, auth)
	if err != nil {
		t.Fatal(err)
	}

	tc.ProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"googleappsscript": providerserver.NewProtocol6WithError(&googleAppsScriptProvider{version: "test", httpClient: httpClient}),
	}
	tc.CheckDestroy = checkDriveFilesTrashed(c)
	resource.UnitTest(t, tc)
}

func checkDriveFilesTrashed(c *client) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "googleappsscript_project" && rs.Type != "googleappsscript_drive_file" {
				continue
			}
			f, err := c.drive.Files.Get(rs.Primary.ID).Fields("trashed").SupportsAllDrives(true).Do()
			if isNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !f.Trashed {
				return fmt.Errorf("%s %s still exists", rs.Type, rs.Primary.ID)
			}
		}
		return nil
	}
}
