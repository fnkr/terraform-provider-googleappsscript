package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/script/v1"
)

const manifestFile = "appsscript.json"

var (
	_ resource.ResourceWithConfigure      = &projectResource{}
	_ resource.ResourceWithImportState    = &projectResource{}
	_ resource.ResourceWithValidateConfig = &projectResource{}
)

type projectResource struct {
	client *client
}

type projectModel struct {
	ID       types.String `tfsdk:"id"`
	Title    types.String `tfsdk:"title"`
	ParentID types.String `tfsdk:"parent_id"`
	Files    types.Map    `tfsdk:"files"`
}

func newProjectResource() resource.Resource {
	return &projectResource{}
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An Apps Script project and its source files. " +
			"Destroying a standalone project moves it to the Drive trash; " +
			"container-bound projects are only removed from state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Script ID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"title": schema.StringAttribute{
				Description: "Project title.",
				Required:    true,
			},
			"parent_id": schema.StringAttribute{
				Description:   "Drive ID of a Google Doc, Sheet, Form or Slides file to bind the script to.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"files": schema.MapAttribute{
				Description: "Source files keyed by file name. Names must end with `.gs`, `.html` or `.json`, " +
					"and `appsscript.json` (the manifest) is required. Use `/` in names for folders.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, resp)
}

func (r *projectResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var files types.Map
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("files"), &files)...)
	if files.IsNull() || files.IsUnknown() {
		return
	}
	for name := range files.Elements() {
		if _, err := toAPIFile(name, ""); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("files").AtMapKey(name), "Invalid file name", err.Error())
		}
	}
	if _, ok := files.Elements()[manifestFile]; !ok {
		resp.Diagnostics.AddAttributeError(path.Root("files"), "Missing manifest", "files must contain "+manifestFile+".")
	}
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.script.Projects.Create(&script.CreateProjectRequest{
		Title:    plan.Title.ValueString(),
		ParentId: plan.ParentID.ValueString(),
	}).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to create project", err.Error())
		return
	}
	plan.ID = types.StringValue(p.ScriptId)
	// Save early so a failed content upload taints the resource instead of orphaning it.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	resp.Diagnostics.Append(r.updateContent(ctx, plan)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()

	p, err := r.client.script.Projects.Get(id).Context(ctx).Do()
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read project", err.Error())
		return
	}
	content, err := r.client.script.Projects.GetContent(id).Context(ctx).Do()
	if err != nil {
		resp.Diagnostics.AddError("Failed to read project content", err.Error())
		return
	}

	prior := map[string]string{}
	if !state.Files.IsNull() {
		resp.Diagnostics.Append(state.Files.ElementsAs(ctx, &prior, false)...)
	}
	files := map[string]string{}
	for _, f := range content.Files {
		name, err := fromAPIFile(f)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read project content", err.Error())
			return
		}
		files[name] = f.Source
		// Keep the configured formatting if the API reformatted JSON.
		if old, ok := prior[name]; ok && f.Type == "JSON" && jsonEqual(old, f.Source) {
			files[name] = old
		}
	}

	state.Title = types.StringValue(p.Title)
	if p.ParentId != "" {
		state.ParentID = types.StringValue(p.ParentId)
	}
	filesValue, diags := types.MapValueFrom(ctx, types.StringType, files)
	resp.Diagnostics.Append(diags...)
	state.Files = filesValue
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Title.Equal(state.Title) {
		_, err := r.client.drive.Files.Update(plan.ID.ValueString(), &drive.File{Name: plan.Title.ValueString()}).
			SupportsAllDrives(true).Context(ctx).Do()
		if err != nil {
			resp.Diagnostics.AddError("Failed to rename project", err.Error())
			return
		}
	}
	if !plan.Files.Equal(state.Files) {
		resp.Diagnostics.Append(r.updateContent(ctx, plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.ParentID.IsNull() {
		resp.Diagnostics.AddWarning("Container-bound project not deleted",
			"The Apps Script API cannot delete container-bound projects. It was removed from Terraform state only.")
		return
	}
	_, err := r.client.drive.Files.Update(state.ID.ValueString(), &drive.File{Trashed: true}).
		SupportsAllDrives(true).Context(ctx).Do()
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete project", err.Error())
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *projectResource) updateContent(ctx context.Context, plan projectModel) (diags diag.Diagnostics) {
	files := map[string]string{}
	diags.Append(plan.Files.ElementsAs(ctx, &files, false)...)
	if diags.HasError() {
		return diags
	}

	// Sort for a deterministic file order, with the manifest first.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == manifestFile || names[j] == manifestFile {
			return names[i] == manifestFile
		}
		return names[i] < names[j]
	})

	content := &script.Content{}
	for _, name := range names {
		f, err := toAPIFile(name, files[name])
		if err != nil {
			diags.AddError("Invalid file", err.Error())
			return diags
		}
		content.Files = append(content.Files, f)
	}
	if _, err := r.client.script.Projects.UpdateContent(plan.ID.ValueString(), content).Context(ctx).Do(); err != nil {
		diags.AddError("Failed to update project content", err.Error())
	}
	return diags
}
