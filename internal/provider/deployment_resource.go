package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/api/script/v1"
)

var (
	_ resource.ResourceWithConfigure   = &deploymentResource{}
	_ resource.ResourceWithImportState = &deploymentResource{}
)

type deploymentResource struct {
	client *client
}

type deploymentModel struct {
	ID            types.String `tfsdk:"id"`
	ScriptID      types.String `tfsdk:"script_id"`
	VersionNumber types.Int64  `tfsdk:"version_number"`
	Description   types.String `tfsdk:"description"`
	WebAppURL     types.String `tfsdk:"web_app_url"`
}

func newDeploymentResource() resource.Resource {
	return &deploymentResource{}
}

func (r *deploymentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (r *deploymentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A deployment of a project version, e.g. as web app, API executable, add-on or library.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Deployment ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"script_id": schema.StringAttribute{
				Description:   "Script ID of the project.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"version_number": schema.Int64Attribute{
				Description: "Version to deploy.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Deployment description.",
				Optional:    true,
			},
			"web_app_url": schema.StringAttribute{
				Description: "Web app URL, if the manifest configures a web app.",
				Computed:    true,
			},
		},
	}
}

func (r *deploymentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, resp)
}

func (r *deploymentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deploymentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.script.Projects.Deployments.Create(plan.ScriptID.ValueString(), plan.config()).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create deployment", err.Error())
		return
	}
	plan.setComputed(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if err := r.waitForVersion(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Failed to create deployment", err.Error())
	}
}

func (r *deploymentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deploymentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.script.Projects.Deployments.Get(state.ScriptID.ValueString(), state.ID.ValueString()).Context(ctx).Do()
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read deployment", err.Error())
		return
	}
	if c := d.DeploymentConfig; c != nil {
		state.VersionNumber = types.Int64Value(c.VersionNumber)
		if c.Description != "" || !state.Description.IsNull() {
			state.Description = types.StringValue(c.Description)
		}
	}
	state.setComputed(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *deploymentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan deploymentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.script.Projects.Deployments.Update(plan.ScriptID.ValueString(), plan.ID.ValueString(),
		&script.UpdateDeploymentRequest{DeploymentConfig: plan.config()}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to update deployment", err.Error())
		return
	}
	plan.setComputed(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if err := r.waitForVersion(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Failed to update deployment", err.Error())
	}
}

func (r *deploymentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deploymentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.script.Projects.Deployments.Delete(state.ScriptID.ValueString(), state.ID.ValueString()).Context(ctx).Do()
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete deployment", err.Error())
	}
}

func (r *deploymentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scriptID, deploymentID, ok := strings.Cut(req.ID, "/")
	if !ok || scriptID == "" || deploymentID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected {script_id}/{deployment_id}, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("script_id"), scriptID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), deploymentID)...)
}

// waitForVersion waits until reads consistently return the deployed version.
func (r *deploymentResource) waitForVersion(ctx context.Context, m deploymentModel) error {
	return waitForStable(ctx, func() (bool, error) {
		d, err := r.client.script.Projects.Deployments.Get(m.ScriptID.ValueString(), m.ID.ValueString()).Context(ctx).Do()
		if isNotFound(err) {
			return false, nil
		}
		return err == nil && d.DeploymentConfig != nil && d.DeploymentConfig.VersionNumber == m.VersionNumber.ValueInt64(), err
	})
}

func (m *deploymentModel) config() *script.DeploymentConfig {
	return &script.DeploymentConfig{
		ScriptId:         m.ScriptID.ValueString(),
		VersionNumber:    m.VersionNumber.ValueInt64(),
		Description:      m.Description.ValueString(),
		ManifestFileName: "appsscript",
	}
}

func (m *deploymentModel) setComputed(d *script.Deployment) {
	m.ID = types.StringValue(d.DeploymentId)
	m.WebAppURL = types.StringNull()
	for _, e := range d.EntryPoints {
		if e.WebApp != nil {
			m.WebAppURL = types.StringValue(e.WebApp.Url)
		}
	}
}
