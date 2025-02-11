package radius

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
)

type environmentResourceModel struct {
	ID        types.String               `tfsdk:"id"`
	Name      types.String               `tfsdk:"name"`
	Simulated types.Bool                 `tfsdk:"simulated"`
	Compute   *environmentComputeModel   `tfsdk:"compute"`
	Providers *environmentProvidersModel `tfsdk:"providers"`
	Recipes   map[string]recipeModel     `tfsdk:"recipes"`
}

type environmentComputeModel struct {
	Kind       types.String          `tfsdk:"kind"`
	ResourceID types.String          `tfsdk:"resource_id"`
	Namespace  types.String          `tfsdk:"namespace"`
	Identity   *computeIdentityModel `tfsdk:"identity"`
}

type computeIdentityModel struct {
	Kind       types.String `tfsdk:"kind"`
	OIDCIssuer types.String `tfsdk:"oidc_issuer"`
	Resource   types.String `tfsdk:"resource"`
}

type environmentProvidersModel struct {
	Azure *providerAzureModel `tfsdk:"azure"`
	AWS   *providerAwsModel   `tfsdk:"aws"`
}

type providerAzureModel struct {
	Scope types.String `tfsdk:"scope"`
}

type providerAwsModel struct {
	Scope types.String `tfsdk:"scope"`
}

type recipeModel struct {
	TemplatePath    types.String            `tfsdk:"template_path"`
	TemplateKind    types.String            `tfsdk:"template_kind"`
	TemplateVersion types.String            `tfsdk:"template_version"`
	Parameters      map[string]types.String `tfsdk:"parameters"`
}

var (
	_ resource.Resource              = &environmentResource{}
	_ resource.ResourceWithConfigure = &environmentResource{}
)

func NewEnvironmentResource() resource.Resource {
	return &environmentResource{}
}

type environmentResource struct {
	client *clients.UCPApplicationsManagementClient
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan environmentResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	environmentResource := &v20231001preview.EnvironmentResource{
		Name:     to.Ptr(plan.Name.ValueString()),
		Location: to.Ptr("global"),
		Properties: &v20231001preview.EnvironmentProperties{
			// TODO: This is KubernetesCompute for now, but it should be a polymorphic type.
			Compute: &v20231001preview.KubernetesCompute{
				Kind:      to.Ptr(plan.Compute.Kind.ValueString()),
				Namespace: to.Ptr(plan.Compute.Namespace.ValueString()),
				// Identity: &v20231001preview.IdentitySettings{
				// 	Kind:       to.Ptr(v20231001preview.IdentitySettingKindAzureComWorkload),
				// 	OidcIssuer: to.Ptr(plan.Compute.Identity.OIDCIssuer.ValueString()),
				// 	Resource:   to.Ptr(plan.Compute.Identity.Resource.ValueString()),
				// },
				// ResourceID: to.Ptr(plan.Compute.ResourceID.ValueString()),
			},

			Providers: &v20231001preview.Providers{
				Azure: &v20231001preview.ProvidersAzure{
					Scope: to.Ptr(plan.Providers.Azure.Scope.ValueString()),
				},
				Aws: &v20231001preview.ProvidersAws{
					Scope: to.Ptr(plan.Providers.AWS.Scope.ValueString()),
				},
			},

			Simulated: to.Ptr(plan.Simulated.ValueBool()),
		},
	}

	// // Add Recipes if configured
	// if len(plan.Recipes) > 0 {
	// 	recipes := make(map[string]v20231001preview.RecipePropertiesClassification)
	// 	for name, recipe := range plan.Recipes {
	// 		if recipe.TemplateKind.ValueString() == "bicep" {
	// 			addRecipe := &v20231001preview.BicepRecipeProperties{
	// 				TemplateKind: to.Ptr(recipe.TemplateKind.ValueString()),
	// 				TemplatePath: to.Ptr(recipe.TemplatePath.ValueString()),
	// 				Parameters:   make(map[string]any),
	// 				PlainHTTP:    to.Ptr(false),
	// 			}

	// 			if recipe.Parameters != nil {
	// 				for k, v := range recipe.Parameters {
	// 					addRecipe.Parameters[k] = v.ValueString()
	// 				}
	// 			}

	// 			recipes[name] = addRecipe
	// 		}

	// 		if recipe.TemplateKind.ValueString() == "terraform" {
	// 			addRecipe := &v20231001preview.TerraformRecipeProperties{
	// 				TemplateKind: to.Ptr(recipe.TemplateKind.ValueString()),
	// 				TemplatePath: to.Ptr(recipe.TemplatePath.ValueString()),
	// 				Parameters:   make(map[string]any),
	// 			}

	// 			if recipe.Parameters != nil {
	// 				for k, v := range recipe.Parameters {
	// 					addRecipe.Parameters[k] = v.ValueString()
	// 				}
	// 			}

	// 			recipes[name] = addRecipe
	// 		}
	// 	}

	// 	environmentResource.Properties.Recipes = map[string]map[string]v20231001preview.RecipePropertiesClassification{
	// 		"recipes": recipes,
	// 	}
	// }

	err := r.client.CreateOrUpdateEnvironment(ctx, *environmentResource.Name, environmentResource)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating environment",
			"Could not create environment, unexpected error: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.AddWarning(
		"Environment created",
		"Environment "+*environmentResource.Name+" has been created.",
	)

	// Use the resourceID for now...
	plan.ID = types.StringValue(*environmentResource.Name)
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *environmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		resp.Diagnostics.AddWarning(
			"Missing Data Source Configuration",
			"Expected a *clients.UCPApplicationsManagementClient in the ProviderData field. Please report this issue to the provider developers.",
		)

		client, err := GetClient(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Failed to create client", err.Error())
			return
		}

		req.ProviderData = client
	}

	client, ok := req.ProviderData.(*clients.UCPApplicationsManagementClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *clients.UCPApplicationsManagementClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *environmentResource) Delete(_ context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	panic("unimplemented")
}

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state environmentResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed environment value from Radius
	environment, err := r.client.GetEnvironment(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Radius Environment",
			"Could not read Radius environment ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Map response body to model
	state.Name = types.StringValue(*environment.Name)
	state.Simulated = types.BoolValue(*environment.Properties.Simulated)

	// Map Compute
	if environment.Properties.Compute != nil {
		if kubernetesCompute, ok := environment.Properties.Compute.(*v20231001preview.KubernetesCompute); ok {
			state.Compute = &environmentComputeModel{
				Kind:       types.StringValue(*kubernetesCompute.Kind),
				ResourceID: types.StringValue(*kubernetesCompute.ResourceID),
				Namespace:  types.StringValue(*kubernetesCompute.Namespace),
			}

			// Map Identity if present
			if kubernetesCompute.Identity != nil {
				state.Compute.Identity = &computeIdentityModel{
					Kind:       types.StringValue(string(*kubernetesCompute.Identity.Kind)),
					OIDCIssuer: types.StringValue(*kubernetesCompute.Identity.OidcIssuer),
					Resource:   types.StringValue(*kubernetesCompute.Identity.Resource),
				}
			}
		}
	}

	// Map Providers if present
	if environment.Properties.Providers != nil {
		state.Providers = &environmentProvidersModel{}

		if environment.Properties.Providers.Azure != nil {
			state.Providers.Azure = &providerAzureModel{
				Scope: types.StringValue(*environment.Properties.Providers.Azure.Scope),
			}
		}

		if environment.Properties.Providers.Aws != nil {
			state.Providers.AWS = &providerAwsModel{
				Scope: types.StringValue(*environment.Properties.Providers.Aws.Scope),
			}
		}
	}

	// Map Recipes if present
	if environment.Properties.Recipes != nil {
		if recipes, ok := environment.Properties.Recipes["recipes"]; ok {
			state.Recipes = make(map[string]recipeModel)
			for name, recipe := range recipes {
				rm := recipeModel{
					Parameters: make(map[string]types.String),
				}

				switch r := recipe.(type) {
				case *v20231001preview.BicepRecipeProperties:
					rm.TemplateKind = types.StringValue(*r.TemplateKind)
					rm.TemplatePath = types.StringValue(*r.TemplatePath)
					for k, v := range r.Parameters {
						if strVal, ok := v.(string); ok {
							rm.Parameters[k] = types.StringValue(strVal)
						}
					}

				case *v20231001preview.TerraformRecipeProperties:
					rm.TemplateKind = types.StringValue(*r.TemplateKind)
					rm.TemplatePath = types.StringValue(*r.TemplatePath)
					for k, v := range r.Parameters {
						if strVal, ok := v.(string); ok {
							rm.Parameters[k] = types.StringValue(strVal)
						}
					}
				}

				state.Recipes[name] = rm
			}
		}
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *environmentResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Radius environment resource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Environment name",
			},
			"simulated": schema.BoolAttribute{
				Optional:    true,
				Description: "Simulated environment",
			},
			"compute": schema.SingleNestedAttribute{
				Required:    true,
				Description: "The compute resource used by application environment",
				Attributes: map[string]schema.Attribute{
					"kind": schema.StringAttribute{
						Required:    true,
						Description: "The Kubernetes compute kind",
						Validators: []validator.String{
							stringvalidator.OneOf("kubernetes"),
						},
					},
					"resource_id": schema.StringAttribute{
						Optional:    true,
						Description: "The resource id of the compute resource for application environment",
					},
					"namespace": schema.StringAttribute{
						Required:    true,
						Description: "The namespace to use for the environment",
					},
					"identity": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "Configuration for supported external identity providers",
						Attributes: map[string]schema.Attribute{
							"kind": schema.StringAttribute{
								Required:    true,
								Description: "kind of identity setting",
								Validators: []validator.String{
									stringvalidator.OneOf("undefined", "azure.com.workload"),
								},
							},
							"oidc_issuer": schema.StringAttribute{
								Optional:    true,
								Description: "The URI for your compute platform's OIDC issuer",
							},
							"resource": schema.StringAttribute{
								Optional:    true,
								Description: "The resource ID of the provisioned identity",
							},
						},
					},
				},
			},
			"providers": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Cloud providers configuration for the environment",
				Attributes: map[string]schema.Attribute{
					"azure": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "The Azure cloud provider configuration",
						Attributes: map[string]schema.Attribute{
							"scope": schema.StringAttribute{
								Required:    true,
								Description: "Target scope for Azure resources to be deployed into",
							},
						},
					},
					"aws": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "The AWS cloud provider configuration",
						Attributes: map[string]schema.Attribute{
							"scope": schema.StringAttribute{
								Required:    true,
								Description: "Target scope for AWS resources to be deployed into",
							},
						},
					},
				},
			},
			"recipes": schema.MapNestedAttribute{
				Optional:    true,
				Description: "Specifies Recipes linked to the Environment",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"template_path": schema.StringAttribute{
							Required:    true,
							Description: "Path to the template provided by the recipe",
						},
						"template_kind": schema.StringAttribute{
							Required:    true,
							Description: "Format of the template provided by the recipe",
							Validators: []validator.String{
								stringvalidator.OneOf("bicep", "terraform"),
							},
						},
						"template_version": schema.StringAttribute{
							Optional:    true,
							Description: "Version of the template to deploy",
						},
						"parameters": schema.MapAttribute{
							Optional:    true,
							Description: "Key/value parameters to pass to the recipe template at deployment",
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (r *environmentResource) Update(_ context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	panic("unimplemented")
}
