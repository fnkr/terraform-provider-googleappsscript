package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/script/v1"
)

// Required OAuth scopes. User credentials must have been granted these at login.
var scopes = []string{
	script.ScriptProjectsScope,
	script.ScriptDeploymentsScope,
	drive.DriveScope,
}

type client struct {
	script *script.Service
	drive  *drive.Service
}

func newClient(ctx context.Context, opts ...option.ClientOption) (*client, error) {
	s, err := script.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	d, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &client{script: s, drive: d}, nil
}

type googleAppsScriptProvider struct {
	version string
	// httpClient replaces authentication and transport in tests.
	httpClient *http.Client
}

type providerModel struct {
	Credentials types.String `tfsdk:"credentials"`
	AccessToken types.String `tfsdk:"access_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &googleAppsScriptProvider{version: version}
	}
}

func (p *googleAppsScriptProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "googleappsscript"
	resp.Version = p.version
}

func (p *googleAppsScriptProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Google Apps Script projects, versions and deployments, " +
			"including scripts bound to Google Docs, Sheets, Forms and Slides.",
		Attributes: map[string]schema.Attribute{
			"credentials": schema.StringAttribute{
				Description: "Path to or contents of an `authorized_user` credentials JSON file. " +
					"Can also be set with `GOOGLE_CREDENTIALS`. Defaults to Application Default Credentials.",
				Optional:  true,
				Sensitive: true,
			},
			"access_token": schema.StringAttribute{
				Description: "OAuth 2.0 access token. Takes precedence over `credentials`. " +
					"Can also be set with `GOOGLE_OAUTH_ACCESS_TOKEN`.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}

func (p *googleAppsScriptProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Clients outlive this request; its context is cancelled once Configure returns.
	ctx = context.WithoutCancel(ctx)
	auth := option.WithHTTPClient(p.httpClient)
	if p.httpClient == nil {
		var err error
		auth, err = clientOption(ctx,
			valueOrEnv(cfg.AccessToken, "GOOGLE_OAUTH_ACCESS_TOKEN"),
			valueOrEnv(cfg.Credentials, "GOOGLE_CREDENTIALS"),
		)
		if err != nil {
			resp.Diagnostics.AddError("Invalid credentials", err.Error())
			return
		}
	}

	c, err := newClient(ctx, auth, option.WithUserAgent("terraform-provider-googleappsscript/"+p.version))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API clients", err.Error())
		return
	}
	resp.ResourceData = c
}

func (p *googleAppsScriptProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newProjectResource,
		newVersionResource,
		newDeploymentResource,
		newDriveFileResource,
	}
}

func (p *googleAppsScriptProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func valueOrEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func clientOption(ctx context.Context, accessToken, credentials string) (option.ClientOption, error) {
	if accessToken != "" {
		return option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})), nil
	}
	if credentials == "" {
		creds, err := google.FindDefaultCredentials(ctx, scopes...)
		if err != nil {
			return nil, err
		}
		return option.WithCredentials(creds), nil
	}

	data := []byte(credentials)
	if !strings.HasPrefix(strings.TrimSpace(credentials), "{") {
		var err error
		if data, err = os.ReadFile(credentials); err != nil {
			return nil, err
		}
	}
	var f struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	// The Apps Script API does not support service accounts.
	if f.Type != string(google.AuthorizedUser) {
		return nil, fmt.Errorf("unsupported credentials type %q, expected %q", f.Type, google.AuthorizedUser)
	}
	creds, err := google.CredentialsFromJSONWithType(ctx, data, google.AuthorizedUser, scopes...)
	if err != nil {
		return nil, err
	}
	return option.WithCredentials(creds), nil
}
