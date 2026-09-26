package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/api/script/v1"
)

var _ resource.ResourceWithConfigure = &versionResource{}

type versionResource struct {
	client *client
}

type versionModel struct {
	ID            types.String `tfsdk:"id"`
	ScriptID      types.String `tfsdk:"script_id"`
	Description   types.String `tfsdk:"description"`
	VersionNumber types.Int64  `tfsdk:"version_number"`
}

func newVersionResource() resource.Resource {
	return &versionResource{}
}

func (r *versionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_version"
}

func (r *versionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An immutable snapshot of a project's current content. " +
			"The Apps Script API cannot delete versions, so destroying only removes it from state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "`{script_id}/{version_number}`.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"script_id": schema.StringAttribute{
				Description:   "Script ID of the project.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Description:   "Version description.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"version_number": schema.Int64Attribute{
				Description:   "Version number.",
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *versionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, resp)
}

func (r *versionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan versionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.script.Projects.Versions.Create(plan.ScriptID.ValueString(), &script.Version{
		Description: plan.Description.ValueString(),
	}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create version", err.Error())
		return
	}
	plan.VersionNumber = types.Int64Value(v.VersionNumber)
	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", v.ScriptId, v.VersionNumber))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read keeps the prior state: versions are immutable and cannot be deleted via
// the API, and reads of new versions flap between found and not found for a while.
func (r *versionResource) Read(context.Context, resource.ReadRequest, *resource.ReadResponse) {}

// Update is never called: all configurable attributes require replacement.
func (r *versionResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {}

// Delete is a no-op: the Apps Script API does not support deleting versions.
func (r *versionResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}
