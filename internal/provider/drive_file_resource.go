package provider

import (
	"context"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/api/drive/v3"
)

// Drive file types that can contain a bound script, as suffixes of their MIME type.
var driveFileTypes = []string{"document", "form", "presentation", "spreadsheet"}

const googleAppsMimeTypePrefix = "application/vnd.google-apps."

const driveFileFields = "id,name,mimeType,trashed,webViewLink"

var (
	_ resource.ResourceWithConfigure   = &driveFileResource{}
	_ resource.ResourceWithImportState = &driveFileResource{}
)

type driveFileResource struct {
	client *client
}

type driveFileModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
	URL  types.String `tfsdk:"url"`
}

func newDriveFileResource() resource.Resource {
	return &driveFileResource{}
}

func (r *driveFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_drive_file"
}

func (r *driveFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An empty Google Doc, Sheet, Form or Slides file to bind a script to via " +
			"`googleappsscript_project.parent_id`. Its content is not managed. " +
			"Destroying it moves it, including bound scripts, to the Drive trash.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Drive file ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "File name.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description:   "One of `" + strings.Join(driveFileTypes, "`, `") + "`.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf(driveFileTypes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				Description:   "URL to open the file in the browser.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *driveFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, resp)
}

func (r *driveFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan driveFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	f, err := r.client.drive.Files.Create(&drive.File{
		Name:     plan.Name.ValueString(),
		MimeType: googleAppsMimeTypePrefix + plan.Type.ValueString(),
	}).Fields(driveFileFields).SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create file", err.Error())
		return
	}
	plan.ID = types.StringValue(f.Id)
	plan.URL = types.StringValue(f.WebViewLink)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *driveFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state driveFileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	f, err := r.client.drive.Files.Get(state.ID.ValueString()).Fields(driveFileFields).SupportsAllDrives(true).Context(ctx).Do()
	if isNotFound(err) || err == nil && f.Trashed {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read file", err.Error())
		return
	}
	typ, ok := strings.CutPrefix(f.MimeType, googleAppsMimeTypePrefix)
	if !ok || !slices.Contains(driveFileTypes, typ) {
		resp.Diagnostics.AddError("Unsupported file", "File "+f.Id+" has unsupported MIME type "+f.MimeType+".")
		return
	}

	state.Name = types.StringValue(f.Name)
	state.Type = types.StringValue(typ)
	state.URL = types.StringValue(f.WebViewLink)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *driveFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan driveFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.drive.Files.Update(plan.ID.ValueString(), &drive.File{Name: plan.Name.ValueString()}).
		SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to rename file", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *driveFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state driveFileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.drive.Files.Update(state.ID.ValueString(), &drive.File{Trashed: true}).
		SupportsAllDrives(true).Context(ctx).Do()
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete file", err.Error())
	}
}

func (r *driveFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
