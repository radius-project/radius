/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-openapi/jsonpointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	rp_util "github.com/project-radius/radius/pkg/rp/util"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/linkrp/frontend/deployment github.com/project-radius/radius/pkg/linkrp/frontend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource v1.ResourceDataModel) (renderers.RendererOutput, error)
	Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (rpv1.DeploymentOutput, error)
	Delete(ctx context.Context, id resources.ID, outputResources []rpv1.OutputResource) error
	FetchSecrets(ctx context.Context, resource ResourceData) (map[string]any, error)
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, sp dataprovider.DataStorageProvider, secretClient sv.SecretValueClient, k8s client.Client) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, sp: sp, secretClient: secretClient, k8s: k8s}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel     model.ApplicationModel
	sp           dataprovider.DataStorageProvider
	secretClient sv.SecretValueClient
	k8s          client.Client
}
type ResourceData struct {
	ID              resources.ID
	Resource        v1.ResourceDataModel
	OutputResources []rpv1.OutputResource
	ComputedValues  map[string]any
	SecretValues    map[string]rpv1.SecretValueReference
}

type EnvironmentMetadata struct {
	Namespace          string
	RecipeLinkType     string
	RecipeTemplatePath string
	RecipeParameters   map[string]any
	Providers          coreDatamodel.Providers
}

func (dp *deploymentProcessor) Render(ctx context.Context, id resources.ID, resource v1.ResourceDataModel) (renderers.RendererOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Rendering resource")

	renderer, err := dp.getResourceRenderer(id)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// fetch the environment ID and recipe name from the link resource
	basicResource, recipe, err := dp.getMetadataFromResource(ctx, id, resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Fetch the environment namespace, recipe's linkType, templatePath and parameters by doing a db lookup
	envMetadata, err := dp.getEnvironmentMetadata(ctx, basicResource.Environment, recipe.Name, resource.ResourceTypeName())
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	kubeNamespace := envMetadata.Namespace
	// Override environment-scope namespace with application-scope kubernetes namespace.
	if basicResource.Application != "" {
		app := &coreDatamodel.Application{}
		if err := rp_util.FetchScopeResource(ctx, dp.sp, basicResource.Application, app); err != nil {
			return renderers.RendererOutput{}, err
		}
		c := app.Properties.Status.Compute
		if c != nil && c.Kind == rpv1.KubernetesComputeKind {
			kubeNamespace = c.KubernetesCompute.Namespace
		}
	}

	// create the context object to be passed to the recipe deployment
	recipeContext, err := handlers.CreateRecipeContextParameter(id.String(), basicResource.Environment, envMetadata.Namespace, basicResource.Application, kubeNamespace)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererOutput, err := renderer.Render(ctx, resource, renderers.RenderOptions{
		Namespace: kubeNamespace,
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe:    recipe,
			LinkType:      envMetadata.RecipeLinkType,
			TemplatePath:  envMetadata.RecipeTemplatePath,
			EnvParameters: envMetadata.RecipeParameters,
		},
		EnvironmentProviders: envMetadata.Providers,
	})
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererOutput.RecipeContext = *recipeContext
	// Check if the output resources have the corresponding provider supported in Radius
	for _, or := range rendererOutput.Resources {
		if or.ResourceType.Provider == "" {
			err = fmt.Errorf("output resource %q does not have a provider specified", or.LocalID)
			return renderers.RendererOutput{}, err
		}
		if !dp.appmodel.IsProviderSupported(or.ResourceType.Provider) {
			return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type))
		}
	}

	return rendererOutput, nil
}

func (dp *deploymentProcessor) getResourceRenderer(id resources.ID) (renderers.Renderer, error) {
	radiusResourceModel, err := dp.appmodel.LookupRadiusResourceModel(id.Type()) // Lookup using resource type
	if err != nil {
		// Internal error: A resource type with unsupported app model shouldn't have reached here
		return nil, err
	}

	return radiusResourceModel.Renderer, nil
}

// Deploys rendered output resources in order of dependencies
// returns updated outputresource properties and computed values
func (dp *deploymentProcessor) Deploy(ctx context.Context, resourceID resources.ID, rendererOutput renderers.RendererOutput) (rpv1.DeploymentOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	// Deploy
	logger.Info("Deploying radius resource")

	// Deploy recipe
	recipeResponse := &handlers.RecipeResponse{}
	var err error
	if rendererOutput.RecipeData.Name != "" {
		recipeResponse, err = dp.appmodel.GetRecipeModel().RecipeHandler.DeployRecipe(ctx, rendererOutput.RecipeData.RecipeProperties, rendererOutput.EnvironmentProviders, rendererOutput.RecipeContext)
		if err != nil {
			return rpv1.DeploymentOutput{}, err
		}
		rendererOutput.RecipeData.Resources = recipeResponse.Resources
	}

	// Recipe based links
	// - Add deployed recipe resource IDs to output resource
	// - Validate that the resource exists by doing a GET on the resource
	// - Populate expected computed values from response of the GET request.
	//
	// Resource id based links
	// - Validate that the resource exists by doing a GET on the resource
	// - Populate expected computed values from response of the GET request.
	//
	// Dapr links
	// - Validate that the resource exists (if resource id is provided)
	// - Apply dapr spec from output resource
	// - Populate expected computed values from response of the GET request.

	outputResources := []rpv1.OutputResource{}
	if rendererOutput.RecipeData.Name == "" {
		// Not a recipe, use the rendered output resources
		outputResources = append(outputResources, rendererOutput.Resources...)
	} else {
		// This is a recipe, we need to do some processing on the output resources.
		processed, err := dp.processRecipeOutputResources(resourceID, rendererOutput.Resources, rendererOutput.RecipeData)
		if err != nil {
			return rpv1.DeploymentOutput{}, err
		}
		outputResources = append(outputResources, processed...)
	}

	// Now we have the combined set of output resources so we can process them.
	outputResources, err = rpv1.OrderOutputResources(outputResources)
	if err != nil {
		return rpv1.DeploymentOutput{}, err
	}

	computedValues := make(map[string]any)
	for i, outputResource := range outputResources {
		if outputResource.IsRadiusManaged() && rendererOutput.RecipeData.Name == "" {
			return rpv1.DeploymentOutput{}, fmt.Errorf("resources deployed through recipe must be Radius managed")
		}

		deployedComputedValues, err := dp.deployOutputResource(ctx, resourceID, &outputResource, rendererOutput)
		if err != nil {
			return rpv1.DeploymentOutput{}, err
		}

		// Running the handler may update the output resource in place, make sure to store the updates.
		outputResources[i] = outputResource

		// Note: deployedComputedValues will likely have some values for resources that were returned
		// by the renderer, and by empty for other cases.
		for k, computedValue := range deployedComputedValues {
			if computedValue != nil {
				computedValues[k] = computedValue
			}
		}
	}

	// Now we need to update the computed values and secrets. It's intended that the recipe outputs
	// can override whatever the renderer specified.
	//
	// Right now the computed values hold whatever dynamic values the renderer provided ...
	// as long as they match one of the output resources.

	// Update static values provided by the renderer.
	for k, computedValue := range rendererOutput.ComputedValues {
		if computedValue.Value != nil {
			computedValues[k] = computedValue.Value
		}
	}

	// Update computedValues fetched from recipe output. Since this is last, any values set
	// in the recipe will win.
	if recipeResponse.Values != nil {
		for k, computedValue := range recipeResponse.Values {
			if computedValue != nil {
				computedValues[k] = computedValue
			}
		}
	}

	// Now it's time to update the secrets.
	//
	// First we copy secrets provided by the renderer.
	// We need to remove secret references, if they are not applicable.
	secretValues := map[string]rpv1.SecretValueReference{}
	for key, reference := range rendererOutput.SecretValues {
		// IFF this is a recipe, make sure local ID exists in output resources, otherwise discard.
		//
		// For a non-recipe we don't do this check, recipes are the only case where discard things
		// from the renderer. Letting it fail later in the non-recipe case will help catch bugs.
		if rendererOutput.RecipeData.Name != "" && reference.LocalID != "" {
			found := false
			for _, outputResource := range outputResources {
				if outputResource.LocalID == reference.LocalID {
					found = true
					break
				}
			}

			if !found {
				continue // This is the case the comments warned you about! Skip this one.
			}
		}

		// Either a non-recipe, or an output resource created by the renderer for a recipe.
		secretValues[key] = reference
	}

	// Update secrets fetched from recipe output. Since this is last, any secrets set
	// in the recipe will win.
	if recipeResponse.Secrets != nil {
		for key, val := range recipeResponse.Secrets {
			value, ok := val.(string)
			if ok {
				secretValues[key] = rpv1.SecretValueReference{Value: value}
			}
		}
	}

	return rpv1.DeploymentOutput{
		DeployedOutputResources: outputResources,
		ComputedValues:          computedValues,
		SecretValues:            secretValues,
	}, nil
}

func (dp *deploymentProcessor) processRecipeOutputResources(resourceID resources.ID, rendererOutputResources []rpv1.OutputResource, recipeData linkrp.RecipeData) ([]rpv1.OutputResource, error) {
	// Because the design of renderers + value-based recipes we have a fairly complex scenario
	// to deal with to support both the old style (resource binding) and new style (value-based)
	// recipes.
	//
	// Links that are coded to the old style (resource binding) will initialize an output resource
	// and computed values/secrets.
	//
	// Links that are coded to the new style (value-based) will not initialize an output resource
	// and expect the recipe to provide the values/secrets.
	//
	// To *really* make it spicy, this isn't binary. For example the Redis link supports
	// resource binding for Azure resources and value-based for everything else.
	//
	// This is transitional and we have plans to simplify the design.

	// The algorithm to deal with this goes like follows:
	//
	// - Iterate output resources and match them (by type) against the deployed resources
	// - Iterate deployed resources and create output resources (skipping those matched in previous step)
	outputResources := []rpv1.OutputResource{}
	matchedDeployedResources := map[string]bool{}
	for _, outputResource := range rendererOutputResources {
		for _, deployedResourceID := range recipeData.Resources {
			parsedID, err := resources.ParseResource(deployedResourceID)
			if err != nil {
				return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to parse id %q of the resource deployed by recipe %q for resource %q: %s", deployedResourceID, recipeData.Name, resourceID.String(), err.Error()))
			}

			// Since this is a resource we "know" then use the preferred API version.
			identity := resourcemodel.FromUCPID(parsedID, recipeData.APIVersion)

			// We actually want to match the ProviderResourceType here. That's the ARM/UCP type.
			// Since deployedResourceID was parsed from an ARM/UCP Resource ID that's the right comparison.
			if outputResource.ResourceType.Provider == identity.ResourceType.Provider &&
				strings.EqualFold(outputResource.ProviderResourceType, identity.ResourceType.Type) {
				// This is a match!
				//
				// - Make sure we assign the identity
				// - Make sure this output resource gets processed for computed values and secrets
				// - Make sure we don't synthesize an extra output resource for this.
				outputResource.Identity = identity
				outputResources = append(outputResources, outputResource)
				matchedDeployedResources[deployedResourceID] = true
			}
		}
	}

	for i, id := range recipeData.Resources {
		if _, ok := matchedDeployedResources[id]; ok {
			// This was already matched to an output resource
			continue
		}

		parsedID, err := resources.ParseResource(id)
		if err != nil {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to parse id %q of the resource deployed by recipe %q for resource %q: %s", id, recipeData.Name, parsedID.String(), err.Error()))
		}

		// Since this isn't a resource we "know" then ignore the preferred API version
		identity := resourcemodel.FromUCPID(parsedID, "")
		outputResource := rpv1.OutputResource{
			LocalID:       fmt.Sprintf("Resource%d", i), // The dependency sorting code requires unique LocalIDs
			Identity:      identity,
			ResourceType:  *identity.ResourceType,
			RadiusManaged: to.Ptr(true),
		}
		outputResources = append(outputResources, outputResource)
	}

	return outputResources, nil
}

func (dp *deploymentProcessor) deployOutputResource(ctx context.Context, id resources.ID, outputResource *rpv1.OutputResource, rendererOutput renderers.RendererOutput) (computedValues map[string]any, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))

	outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
	if err != nil {
		return nil, err
	}

	resourceIdentity, properties, err := outputResourceModel.ResourceHandler.Put(ctx, outputResource)
	if err != nil {
		return nil, err
	}
	if (resourceIdentity != resourcemodel.ResourceIdentity{}) {
		outputResource.Identity = resourceIdentity
	}

	if outputResource.Identity.ResourceType == nil {
		err = fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler or renderer", outputResource.LocalID)
		return nil, err
	}

	// Values consumed by other Radius resource types through connections
	computedValues = map[string]any{}
	// Copy deployed output resource property values into corresponding expected computed values
	for k, v := range rendererOutput.ComputedValues {
		logger.Info(fmt.Sprintf("Processing computed value for %s", k))
		// A computed value might be a reference to a 'property' returned in preserved properties
		if outputResource.LocalID == v.LocalID && v.PropertyReference != "" {
			computedValues[k] = properties[v.PropertyReference]
			continue
		}

		// A computed value might be a 'pointer' into the deployed resource
		if outputResource.LocalID == v.LocalID && v.JSONPointer != "" {
			logger.Info(fmt.Sprintf("Parsing json pointer %q, output resource local id: %v", v.JSONPointer, outputResource.LocalID))
			pointer, err := jsonpointer.New(v.JSONPointer)
			if err != nil {
				return nil, fmt.Errorf("failed to parse JSON pointer %q for computed value %q for link %q: %w", v.JSONPointer, k, id.String(), err)
			}

			value, _, err := pointer.Get(outputResource.Resource)
			if err != nil {
				return nil, fmt.Errorf("failed to process JSON pointer %q to fetch computed value %q. Output resource identity: %v. Link id: %q: %w", v.JSONPointer, k, outputResource.Identity.Data, id.String(), err)
			}
			computedValues[k] = value
		}
	}

	return computedValues, nil
}

func (dp *deploymentProcessor) Delete(ctx context.Context, id resources.ID, outputResources []rpv1.OutputResource) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	orderedOutputResources, err := rpv1.OrderOutputResources(outputResources)
	if err != nil {
		return err
	}

	// Loop over each output resource and delete in reverse dependency order
	for i := len(orderedOutputResources) - 1; i >= 0; i-- {
		outputResource := orderedOutputResources[i]
		logger.Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %s\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType.Type))
		outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
		if err != nil {
			return err
		}
		err = outputResourceModel.ResourceHandler.Delete(ctx, &outputResource)
		if err != nil {
			return err
		}

	}

	return nil
}

func (dp *deploymentProcessor) FetchSecrets(ctx context.Context, resourceData ResourceData) (map[string]any, error) {
	secretValues := map[string]any{}

	for k, secretReference := range resourceData.SecretValues {
		secret, err := dp.fetchSecret(ctx, resourceData.OutputResources, secretReference)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch secret %s for resource %s: %w", k, resourceData.ID.String(), err)
		}

		if (secretReference.Transformer != resourcemodel.ResourceType{}) {
			outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(secretReference.Transformer)
			if err != nil {
				return nil, err
			} else if outputResourceModel.SecretValueTransformer == nil {
				return nil, fmt.Errorf("could not find a secret transformer for %q", secretReference.Transformer)
			}

			secret, err = outputResourceModel.SecretValueTransformer.Transform(ctx, resourceData.ComputedValues, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to transform secret %s for resource %s: %w", k, resourceData.ID.String(), err)
			}
		}

		secretValues[k] = secret
	}

	return secretValues, nil
}

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, outputResources []rpv1.OutputResource, reference rpv1.SecretValueReference) (any, error) {
	if reference.Value != "" {
		// The secret reference contains the value itself
		return reference.Value, nil
	}

	// Reference to operations to fetch secrets is currently only supported for Azure resources
	if dp.secretClient == nil {
		return nil, errors.New("no Azure credentials provided to fetch secret")
	}

	// Find the output resource that maps to the secret value reference
	for _, outputResource := range outputResources {
		if outputResource.LocalID == reference.LocalID {
			return dp.secretClient.FetchSecret(ctx, outputResource.Identity, reference.Action, reference.ValueSelector)
		}
	}

	return nil, fmt.Errorf("cannot find an output resource matching LocalID for secret reference %s", reference.LocalID)
}

// getMetadataFromResource returns the environment id and the recipe name to look up environment metadata
func (dp *deploymentProcessor) getMetadataFromResource(ctx context.Context, resourceID resources.ID, resource v1.DataModelInterface) (basicResource *rpv1.BasicResourceProperties, recipe linkrp.LinkRecipe, err error) {
	resourceType := strings.ToLower(resourceID.Type())
	switch resourceType {
	case strings.ToLower(linkrp.MongoDatabasesResourceType):
		obj := resource.(*datamodel.MongoDatabase)
		basicResource = &obj.Properties.BasicResourceProperties
		if obj.Properties.Mode == datamodel.LinkModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		obj := resource.(*datamodel.SqlDatabase)
		basicResource = &obj.Properties.BasicResourceProperties
		if obj.Properties.Mode == datamodel.LinkModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(linkrp.RedisCachesResourceType):
		obj := resource.(*datamodel.RedisCache)
		basicResource = &obj.Properties.BasicResourceProperties
		recipe.Name = obj.Properties.Recipe.Name
		recipe.Parameters = obj.Properties.Recipe.Parameters
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		obj := resource.(*datamodel.RabbitMQMessageQueue)
		basicResource = &obj.Properties.BasicResourceProperties
		recipe.Name = obj.Properties.Recipe.Name
		recipe.Parameters = obj.Properties.Recipe.Parameters
	case strings.ToLower(linkrp.ExtendersResourceType):
		obj := resource.(*datamodel.Extender)
		basicResource = &obj.Properties.BasicResourceProperties
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		obj := resource.(*datamodel.DaprStateStore)
		basicResource = &obj.Properties.BasicResourceProperties
		if obj.Properties.Mode == datamodel.LinkModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		obj := resource.(*datamodel.DaprSecretStore)
		basicResource = &obj.Properties.BasicResourceProperties
		if obj.Properties.Mode == datamodel.LinkModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		obj := resource.(*datamodel.DaprPubSubBroker)
		basicResource = &obj.Properties.BasicResourceProperties
		if obj.Properties.Mode == datamodel.LinkModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(linkrp.DaprInvokeHttpRoutesResourceType):
		obj := resource.(*datamodel.DaprInvokeHttpRoute)
		basicResource = &obj.Properties.BasicResourceProperties
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	default:
		// Internal error: this shouldn't happen unless a new supported resource type wasn't added here
		err = fmt.Errorf("unsupported resource type: %q for resource ID: %q", resourceType, resourceID.String())
		basicResource = nil
		return
	}
	return
}

// getEnvironmentMetadata fetches the environment resource from the db to retrieve namespace and recipe metadata required to deploy the link and linked resources ```
func (dp *deploymentProcessor) getEnvironmentMetadata(ctx context.Context, environmentID, recipeName, linkType string) (envMetadata EnvironmentMetadata, err error) {
	env := &coreDatamodel.Environment{}
	if err = rp_util.FetchScopeResource(ctx, dp.sp, environmentID, env); err != nil {
		return
	}

	envMetadata = EnvironmentMetadata{}
	if env.Properties.Compute != (rpv1.EnvironmentCompute{}) && env.Properties.Compute.KubernetesCompute != (rpv1.KubernetesComputeProperties{}) {
		envMetadata.Namespace = env.Properties.Compute.KubernetesCompute.Namespace
	} else {
		return envMetadata, fmt.Errorf("cannot find namespace in the environment resource")
	}
	// identify recipe's template path associated with provided recipe name
	recipe, ok := env.Properties.Recipes[linkType][recipeName]
	if ok {
		envMetadata.RecipeLinkType = linkType
		envMetadata.RecipeTemplatePath = recipe.TemplatePath
		envMetadata.RecipeParameters = recipe.Parameters
	} else if recipeName != "" {
		return envMetadata, fmt.Errorf("recipe with name %q does not exist in the environment %s", recipeName, environmentID)
	}

	// get the providers metadata to deploy the recipe
	envMetadata.Providers = env.Properties.Providers

	return envMetadata, nil
}
