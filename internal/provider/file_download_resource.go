package provider

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkpath "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ resource.Resource                = (*fileDownloadResource)(nil)
	_ resource.ResourceWithConfigure   = (*fileDownloadResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*fileDownloadResource)(nil)
	_ resource.ResourceWithImportState = (*fileDownloadResource)(nil)
)

// NewFileDownloadResource registers the download resource with the provider.
func NewFileDownloadResource() resource.Resource {
	return &fileDownloadResource{}
}

type fileDownloadResource struct {
	client multipasscli.Client
	hostOS string
}

type fileDownloadResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Instance      types.String `tfsdk:"instance"`
	Source        types.String `tfsdk:"source"`
	Destination   types.String `tfsdk:"destination"`
	Recursive     types.Bool   `tfsdk:"recursive"`
	CreateParents types.Bool   `tfsdk:"create_parents"`
	Overwrite     types.Bool   `tfsdk:"overwrite"`
	Triggers      types.Map    `tfsdk:"triggers"`
	ContentHash   types.String `tfsdk:"content_hash"`
}

func (r *fileDownloadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file_download"
}

func (r *fileDownloadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Downloads files or directories from Multipass instances to the host using `multipass transfer`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Identifier in the form `<instance>:<source>-><destination>`.",
				MarkdownDescription: "Identifier in the form `<instance>:<source>-><destination>`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance": schema.StringAttribute{
				Required:            true,
				Description:         "Multipass instance to read from.",
				MarkdownDescription: "Multipass instance to read from.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source": schema.StringAttribute{
				Required:            true,
				Description:         "Path inside the instance to download.",
				MarkdownDescription: "Path inside the instance to download.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				Description:         "Local path where the file or directory should be written.",
				MarkdownDescription: "Local path where the file or directory should be written.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"recursive": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Set true when downloading directories (maps to `multipass transfer --recursive`).",
				MarkdownDescription: "Set true when downloading directories (maps to `multipass transfer --recursive`).",
			},
			"create_parents": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				Description:         "Create local parent directories as needed.",
				MarkdownDescription: "Create local parent directories as needed.",
			},
			"overwrite": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				Description:         "Whether to overwrite existing files/directories at the destination.",
				MarkdownDescription: "Whether to overwrite existing files/directories at the destination.",
			},
			"triggers": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				Description:         "Map of arbitrary values that, when changed, force the download to re-run (similar to `null_resource.triggers`).",
				MarkdownDescription: "Map of arbitrary values that, when changed, force the download to re-run (similar to `null_resource.triggers`).",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"content_hash": schema.StringAttribute{
				Computed:            true,
				Description:         "SHA256 hash of the downloaded payload.",
				MarkdownDescription: "SHA256 hash of the downloaded payload.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *fileDownloadResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data := req.ProviderData.(providerData)
	r.client = data.client
	r.hostOS = data.hostOS
}

func (r *fileDownloadResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan fileDownloadResourceModel
	var state fileDownloadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !mapsEqual(ctx, plan.Triggers, state.Triggers) {
		resp.RequiresReplace = append(resp.RequiresReplace, frameworkpath.Root("triggers"))
	}
}

func (r *fileDownloadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var plan fileDownloadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if diags := r.downloadAndWrite(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s:%s->%s", plan.Instance.ValueString(), plan.Source.ValueString(), plan.Destination.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fileDownloadResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state fileDownloadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Instance.IsNull() || state.Instance.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	if _, err := r.client.GetInstance(ctx, state.Instance.ValueString()); err != nil {
		if err == multipasscli.ErrNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to verify instance", err.Error())
		return
	}

	if _, err := os.Stat(state.Destination.ValueString()); os.IsNotExist(err) {
		resp.Diagnostics.AddWarning("Destination missing", "Local destination is missing; resource will be recreated on next apply.")
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *fileDownloadResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var plan fileDownloadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if diags := r.downloadAndWrite(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fileDownloadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state fileDownloadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dest := state.Destination.ValueString()
	if dest == "" {
		return
	}

	if err := os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		resp.Diagnostics.AddWarning("Failed to remove destination", err.Error())
	}
}

func (r *fileDownloadResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError("Unsupported import", "multipass_file_download resources cannot be imported.")
}

func (r *fileDownloadResource) downloadAndWrite(ctx context.Context, model *fileDownloadResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	dest := filepath.Clean(model.Destination.ValueString())
	if dest == "" {
		diags.AddError("Invalid destination", "Destination must be non-empty")
		return diags
	}

	if r.hostOS == "windows" {
		return r.downloadWithTar(ctx, model, dest)
	}
	return r.downloadDirect(ctx, model, dest)
}

func (r *fileDownloadResource) downloadDirect(ctx context.Context, model *fileDownloadResourceModel, dest string) diag.Diagnostics {
	var diags diag.Diagnostics

	instance := model.Instance.ValueString()
	source := model.Source.ValueString()
	if instance == "" || source == "" {
		diags.AddError("Invalid configuration", "`instance` and `source` must be set")
		return diags
	}

	tempDir, err := os.MkdirTemp("", "multipass-file-download-direct-*")
	if err != nil {
		diags.AddError("Failed to create temp directory", err.Error())
		return diags
	}
	defer os.RemoveAll(tempDir)

	err = r.client.Transfer(ctx, multipasscli.TransferOptions{
		Sources:     []string{fmt.Sprintf("%s:%s", instance, source)},
		Destination: tempDir,
		Recursive:   model.Recursive.ValueBool(),
		Parents:     true,
	})
	if err != nil {
		diags.AddError("Failed to download from instance", err.Error())
		return diags
	}

	if model.Recursive.ValueBool() {
		sourceDir := filepath.Join(tempDir, filepath.Base(source))
		diags.Append(r.copyDirectory(sourceDir, dest, model)...)
		if diags.HasError() {
			return diags
		}
		hashValue, err := hashDirectory(dest)
		if err != nil {
			diags.AddError("Failed to hash directory", err.Error())
			return diags
		}
		model.ContentHash = types.StringValue(hashValue)
		return diags
	}

	targetFile := filepath.Join(tempDir, filepath.Base(source))
	data, err := os.ReadFile(targetFile)
	if err != nil {
		diags.AddError("Failed to read downloaded file", err.Error())
		return diags
	}
	diags.Append(r.writeFileBytes(data, dest, model)...)
	if diags.HasError() {
		return diags
	}
	model.ContentHash = types.StringValue(hashBytes(data))
	return diags
}

func (r *fileDownloadResource) copyDirectory(src, dest string, model *fileDownloadResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if stat, err := os.Stat(dest); err == nil {
		if !stat.IsDir() {
			diags.AddError("Destination exists", fmt.Sprintf("%q exists and is not a directory", dest))
			return diags
		}
		if model.Overwrite.ValueBool() {
			if err := os.RemoveAll(dest); err != nil {
				diags.AddError("Failed to clean destination", err.Error())
				return diags
			}
		} else {
			diags.AddError("Destination exists", fmt.Sprintf("Directory %q already exists and overwrite=false", dest))
			return diags
		}
	}

	if err := ensureParentDir(dest, model.CreateParents.ValueBool()); err != nil {
		diags.AddError("Failed to prepare destination", err.Error())
		return diags
	}

	if err := copyDirContents(src, dest); err != nil {
		diags.AddError("Failed to copy directory", err.Error())
		return diags
	}

	return diags
}

func (r *fileDownloadResource) downloadWithTar(ctx context.Context, model *fileDownloadResourceModel, dest string) diag.Diagnostics {
	var diags diag.Diagnostics

	if model.Recursive.ValueBool() {
		archiveData, d := r.fetchDirectoryTar(ctx, model)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		diags.Append(r.writeDirectoryFromTar(archiveData, dest, model)...)
		if diags.HasError() {
			return diags
		}
		hashValue, err := hashDirectory(dest)
		if err != nil {
			diags.AddError("Failed to hash directory", err.Error())
			return diags
		}
		model.ContentHash = types.StringValue(hashValue)
		return diags
	}

	fileData, d := r.fetchFileBytes(ctx, model)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	diags.Append(r.writeFileBytes(fileData, dest, model)...)
	if diags.HasError() {
		return diags
	}

	model.ContentHash = types.StringValue(hashBytes(fileData))
	return diags
}

func (r *fileDownloadResource) fetchFileBytes(ctx context.Context, model *fileDownloadResourceModel) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	instance := model.Instance.ValueString()
	source := model.Source.ValueString()
	if instance == "" || source == "" {
		diags.AddError("Invalid configuration", "`instance` and `source` must be set")
		return nil, diags
	}

	data, err := r.client.TransferCapture(ctx, multipasscli.TransferOptions{
		Sources:     []string{fmt.Sprintf("%s:%s", instance, source)},
		Destination: "-",
	})
	if err != nil {
		diags.AddError("Failed to download from instance", err.Error())
		return nil, diags
	}
	return data, diags
}

func (r *fileDownloadResource) fetchDirectoryTar(ctx context.Context, model *fileDownloadResourceModel) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	instance := model.Instance.ValueString()
	source := model.Source.ValueString()
	if instance == "" || source == "" {
		diags.AddError("Invalid configuration", "`instance` and `source` must be set")
		return nil, diags
	}

	clean := path.Clean(source)
	baseDir := path.Dir(clean)
	target := path.Base(clean)
	if baseDir == "." {
		baseDir = "/"
	}

	tmpTar := fmt.Sprintf("/tmp/multipass-download-%d.tar", time.Now().UnixNano())
	createCmd := []string{"tar", "-C", baseDir, "-cf", tmpTar, target}
	if err := r.client.Exec(ctx, instance, createCmd); err != nil {
		diags.AddError("Failed to archive remote directory", err.Error())
		return nil, diags
	}
	defer r.client.Exec(ctx, instance, []string{"rm", "-f", tmpTar})

	data, err := r.client.TransferCapture(ctx, multipasscli.TransferOptions{
		Sources:     []string{fmt.Sprintf("%s:%s", instance, tmpTar)},
		Destination: "-",
	})
	if err != nil {
		diags.AddError("Failed to download archive", err.Error())
		return nil, diags
	}
	return data, diags
}

func (r *fileDownloadResource) writeFileBytes(data []byte, dest string, model *fileDownloadResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	destPath := dest
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		destPath = filepath.Join(dest, filepath.Base(model.Source.ValueString()))
	}

	if _, err := os.Stat(destPath); err == nil && !model.Overwrite.ValueBool() {
		diags.AddError("Destination exists", fmt.Sprintf("File %q already exists and overwrite=false", destPath))
		return diags
	}

	if err := ensureParentDir(destPath, model.CreateParents.ValueBool()); err != nil {
		diags.AddError("Failed to prepare destination", err.Error())
		return diags
	}

	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		diags.AddError("Failed to write destination file", err.Error())
		return diags
	}

	return diags
}

func (r *fileDownloadResource) writeDirectoryFromTar(data []byte, dest string, model *fileDownloadResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if stat, err := os.Stat(dest); err == nil {
		if !stat.IsDir() {
			diags.AddError("Destination exists", fmt.Sprintf("%q exists and is not a directory", dest))
			return diags
		}
		if model.Overwrite.ValueBool() {
			if err := os.RemoveAll(dest); err != nil {
				diags.AddError("Failed to clean destination", err.Error())
				return diags
			}
		} else {
			diags.AddError("Destination exists", fmt.Sprintf("Directory %q already exists and overwrite=false", dest))
			return diags
		}
	}

	if err := ensureParentDir(dest, model.CreateParents.ValueBool()); err != nil {
		diags.AddError("Failed to prepare destination", err.Error())
		return diags
	}

	tr := tar.NewReader(bytes.NewReader(data))
	destPrefix := filepath.Clean(dest) + string(os.PathSeparator)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			diags.AddError("Failed to read archive", err.Error())
			return diags
		}

		targetPath, err := sanitizeExtractPath(destPrefix, hdr.Name)
		if err != nil {
			diags.AddError("Invalid archive entry", err.Error())
			return diags
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				diags.AddError("Failed to create directory", err.Error())
				return diags
			}
		case tar.TypeReg:
			if err := ensureParentDir(targetPath, true); err != nil {
				diags.AddError("Failed to create parent directory", err.Error())
				return diags
			}
			out, err := os.Create(targetPath)
			if err != nil {
				diags.AddError("Failed to create file", err.Error())
				return diags
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				diags.AddError("Failed to extract file", err.Error())
				return diags
			}
			out.Close()
		default:
			diags.AddError("Unsupported archive entry", fmt.Sprintf("Entry %q has unsupported type %d", hdr.Name, hdr.Typeflag))
			return diags
		}
	}

	return diags
}

func sanitizeExtractPath(destPrefix, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if strings.Contains(cleanName, "..") {
		return "", fmt.Errorf("archive entry %q contains parent directory traversal", name)
	}
	target := filepath.Join(destPrefix, cleanName)
	if !strings.HasPrefix(target, destPrefix) {
		return "", fmt.Errorf("archive entry %q escapes destination", name)
	}
	return target, nil
}

func ensureParentDir(path string, create bool) error {
	parent := filepath.Dir(path)
	if create {
		return os.MkdirAll(parent, 0o755)
	}
	if _, err := os.Stat(parent); err != nil {
		return fmt.Errorf("parent directory %q does not exist (set create_parents=true to create it)", parent)
	}
	return nil
}

func copyDirContents(src, dest string) error {
	return filepath.WalkDir(src, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, current)
		if err != nil {
			return err
		}

		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := ensureParentDir(target, true); err != nil {
			return err
		}
		return copyFileContents(current, target)
	})
}

func copyFileContents(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func mapsEqual(ctx context.Context, a, b types.Map) bool {
	if a.IsNull() && b.IsNull() {
		return true
	}

	var amap, bmap map[string]string
	if !a.IsNull() {
		if err := a.ElementsAs(ctx, &amap, false); err != nil {
			return false
		}
	}
	if !b.IsNull() {
		if err := b.ElementsAs(ctx, &bmap, false); err != nil {
			return false
		}
	}

	if len(amap) != len(bmap) {
		return false
	}

	for k, v := range amap {
		if bmap[k] != v {
			return false
		}
	}
	return true
}
