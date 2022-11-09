// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprinvokehttproutes"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprpubsubbrokers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprsecretstores"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprstatestores"
	"github.com/project-radius/radius/pkg/linkrp/renderers/extenders"
	"github.com/project-radius/radius/pkg/linkrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/linkrp/renderers/rabbitmqmessagequeues"
	"github.com/project-radius/radius/pkg/linkrp/renderers/rediscaches"
	"github.com/project-radius/radius/pkg/linkrp/renderers/sqldatabases"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/linkrp/frontend/deployment github.com/project-radius/radius/pkg/linkrp/frontend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error)
	Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (DeploymentOutput, error)
	Delete(ctx context.Context, resource ResourceData) error
	FetchSecrets(ctx context.Context, resource ResourceData) (map[string]interface{}, error)
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, sp dataprovider.DataStorageProvider, secretClient rp.SecretValueClient, k8s client.Client) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, sp: sp, secretClient: secretClient, k8s: k8s}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel     model.ApplicationModel
	sp           dataprovider.DataStorageProvider
	secretClient rp.SecretValueClient
	k8s          client.Client
}

type DeploymentOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]interface{}
	SecretValues   map[string]rp.SecretValueReference
	RecipeData     datamodel.RecipeData
}

type ResourceData struct {
	ID              resources.ID
	Resource        conv.DataModelInterface
	OutputResources []outputresource.OutputResource
	ComputedValues  map[string]interface{}
	SecretValues    map[string]rp.SecretValueReference
	RecipeData      datamodel.RecipeData
}

type EnvironmentMetadata struct {
	Namespace          string
	RecipeLinkType     string
	RecipeTemplatePath string
	Providers          coreDatamodel.Providers
}

func (dp *deploymentProcessor) Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldResourceID, id.String())
	logger.Info("Rendering resource")

	renderer, err := dp.getResourceRenderer(id)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// fetch the environment ID and recipe name from the resource
	env, recipe, err := dp.getMetadataFromResource(ctx, id, resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Fetch the environment namespace, recipe link type and recipe template path by doing a db lookup
	envMetadata, err := dp.getEnvironmentMetadata(ctx, env, recipe.Name)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererOutput, err := renderer.Render(ctx, resource, renderers.RenderOptions{
		Namespace: envMetadata.Namespace,
		RecipeProperties: datamodel.RecipeProperties{
			LinkRecipe:   recipe,
			LinkType:     envMetadata.RecipeLinkType,
			TemplatePath: envMetadata.RecipeTemplatePath,
		},
		EnvironmentProviders: envMetadata.Providers,
	})
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Check if the output resources have the corresponding provider supported in Radius
	for _, or := range rendererOutput.Resources {
		if or.ResourceType.Provider == "" {
			err = fmt.Errorf("output resource %q does not have a provider specified", or.LocalID)
			return renderers.RendererOutput{}, err
		}
		if !dp.appmodel.IsProviderSupported(or.ResourceType.Provider) {
			return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type))
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
func (dp *deploymentProcessor) Deploy(ctx context.Context, resourceID resources.ID, rendererOutput renderers.RendererOutput) (DeploymentOutput, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldResourceID, resourceID.String())
	// Deploy
	logger.Info("Deploying radius resource")

	// Deploy recipe
	if rendererOutput.RecipeData.Name != "" {
		deployedRecipeResourceIDs, err := dp.appmodel.GetRecipeModel().RecipeHandler.DeployRecipe(ctx, rendererOutput.RecipeData.RecipeProperties, rendererOutput.EnvironmentProviders)
		if err != nil {
			return DeploymentOutput{}, err
		}
		rendererOutput.RecipeData.Resources = deployedRecipeResourceIDs
	}

	// Order output resources in deployment dependency order
	orderedOutputResources, err := outputresource.OrderOutputResources(rendererOutput.Resources)
	if err != nil {
		return DeploymentOutput{}, err
	}

	// Recipe based links - Add deployed recipe resource IDs to output resource; Validate that the resource exists by doing a GET on the resource; Populate expected computed values from response of the GET request.
	// Resource id based links - Validate that the resource exists by doing a GET on the resource; Populate expected computed values from response of the GET request.
	// Dapr links - Validate that the resource exists (if resource id is provided); Apply dapr spec from output resource; Populate expected computed values from response of the GET request.
	updatedOutputResources := []outputresource.OutputResource{}
	computedValues := make(map[string]interface{})
	for _, outputResource := range orderedOutputResources {
		// Add resources deployed by recipe to output resource identity
		for _, id := range rendererOutput.RecipeData.Resources {
			if rendererOutput.RecipeData.Provider == resourcemodel.ProviderAzure {
				parsedID, err := resources.ParseResource(id)
				if err != nil {
					return DeploymentOutput{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("failed to parse id %q of the resource deployed by recipe %q for resource %q: %s", id, rendererOutput.RecipeData.Name, resourceID.String(), err.Error()))
				}

				if outputResource.ProviderResourceType == parsedID.Type() {
					outputResource.Identity = resourcemodel.NewARMIdentity(&outputResource.ResourceType, id, rendererOutput.RecipeData.APIVersion)
				}
			}
		}

		deployedComputedValues, err := dp.deployOutputResource(ctx, resourceID, &outputResource, rendererOutput)
		if err != nil {
			return DeploymentOutput{}, err
		}

		updatedOutputResources = append(updatedOutputResources, outputResource)

		for k, computedValue := range deployedComputedValues {
			if computedValue != nil {
				computedValues[k] = computedValue
			}
		}
	}

	// Update static values
	for k, computedValue := range rendererOutput.ComputedValues {
		if computedValue.Value != nil {
			computedValues[k] = computedValue.Value
		}
	}

	return DeploymentOutput{
		Resources:      updatedOutputResources,
		ComputedValues: computedValues,
		SecretValues:   rendererOutput.SecretValues,
		RecipeData:     rendererOutput.RecipeData,
	}, nil
}

func (dp *deploymentProcessor) deployOutputResource(ctx context.Context, id resources.ID, outputResource *outputresource.OutputResource, rendererOutput renderers.RendererOutput) (computedValues map[string]interface{}, err error) {
	logger := radlogger.GetLogger(ctx)
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
	computedValues = map[string]interface{}{}
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

func (dp *deploymentProcessor) Delete(ctx context.Context, resourceData ResourceData) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldResourceID, resourceData.ID)

	orderedOutputResources, err := outputresource.OrderOutputResources(resourceData.OutputResources)
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

		if outputResource.IsRadiusManaged() {
			err = outputResourceModel.ResourceHandler.Delete(ctx, &outputResource)
			if err != nil {
				return err
			}
		} else if resourceData.RecipeData.Name != "" {
			// If the resource is not Radius managed for a link tied to a recipe, then this is a bug in the output resource initialization in renderer
			return fmt.Errorf("resources deployed through recipe must be Radius managed")
		}
		logger.Info("Underlying resource lifecycle is not managed by Radius, skipping deletion")
	}

	return nil
}

func (dp *deploymentProcessor) FetchSecrets(ctx context.Context, resourceData ResourceData) (map[string]interface{}, error) {
	secretValues := map[string]interface{}{}

	for k, secretReference := range resourceData.SecretValues {
		secret, err := dp.fetchSecret(ctx, resourceData.OutputResources, secretReference, resourceData.RecipeData)
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

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, outputResources []outputresource.OutputResource, reference rp.SecretValueReference, recipeData datamodel.RecipeData) (interface{}, error) {
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
func (dp *deploymentProcessor) getMetadataFromResource(ctx context.Context, resourceID resources.ID, resource conv.DataModelInterface) (envId string, recipe datamodel.LinkRecipe, err error) {
	resourceType := strings.ToLower(resourceID.Type())
	switch resourceType {
	case strings.ToLower(mongodatabases.ResourceType):
		obj := resource.(*datamodel.MongoDatabase)
		envId = obj.Properties.Environment
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(sqldatabases.ResourceType):
		obj := resource.(*datamodel.SqlDatabase)
		envId = obj.Properties.Environment
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(rediscaches.ResourceType):
		obj := resource.(*datamodel.RedisCache)
		envId = obj.Properties.Environment
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(rabbitmqmessagequeues.ResourceType):
		obj := resource.(*datamodel.RabbitMQMessageQueue)
		envId = obj.Properties.Environment
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(extenders.ResourceType):
		obj := resource.(*datamodel.Extender)
		envId = obj.Properties.Environment
	case strings.ToLower(daprstatestores.ResourceType):
		obj := resource.(*datamodel.DaprStateStore)
		envId = obj.Properties.Environment
		if obj.Properties.Mode == datamodel.DaprStateStoreModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(daprsecretstores.ResourceType):
		obj := resource.(*datamodel.DaprSecretStore)
		envId = obj.Properties.Environment
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(daprpubsubbrokers.ResourceType):
		obj := resource.(*datamodel.DaprPubSubBroker)
		envId = obj.Properties.Environment
		if obj.Properties.Mode == datamodel.DaprPubSubBrokerModeRecipe {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	case strings.ToLower(daprinvokehttproutes.ResourceType):
		obj := resource.(*datamodel.DaprInvokeHttpRoute)
		envId = obj.Properties.Environment
		if obj.Properties.Recipe.Name != "" {
			recipe.Name = obj.Properties.Recipe.Name
			recipe.Parameters = obj.Properties.Recipe.Parameters
		}
	default:
		// Internal error: this shouldn't happen unless a new supported resource type wasn't added here
		return "", recipe, fmt.Errorf("unsupported resource type: %q for resource ID: %q", resourceType, resourceID.String())
	}

	return envId, recipe, nil
}

// getEnvironmentMetadata fetches the environment resource from the db to retrieve namespace and recipe metadata required to deploy the link and linked resources ```
func (dp *deploymentProcessor) getEnvironmentMetadata(ctx context.Context, environmentID string, recipeName string) (envMetadata EnvironmentMetadata, err error) {
	envId, err := resources.ParseResource(environmentID)
	envMetadata = EnvironmentMetadata{}
	if err != nil {
		return envMetadata, conv.NewClientErrInvalidRequest(fmt.Sprintf("provided environment id %q is not a valid id.", environmentID))
	}

	env := &coreDatamodel.Environment{}
	if !strings.EqualFold(envId.Type(), env.ResourceTypeName()) {
		return envMetadata, conv.NewClientErrInvalidRequest(fmt.Sprintf("provided environment id type %q is not a valid type.", envId.Type()))
	}

	sc, err := dp.sp.GetStorageClient(ctx, envId.Type())
	if err != nil {
		return
	}
	res, err := sc.Get(ctx, environmentID)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return envMetadata, conv.NewClientErrInvalidRequest(fmt.Sprintf("environment %q does not exist", environmentID))
		}
		return
	}
	err = res.As(env)
	if err != nil {
		return
	}

	if env.Properties.Compute != (coreDatamodel.EnvironmentCompute{}) && env.Properties.Compute.KubernetesCompute != (coreDatamodel.KubernetesComputeProperties{}) {
		envMetadata.Namespace = env.Properties.Compute.KubernetesCompute.Namespace
	} else {
		return envMetadata, fmt.Errorf("cannot find namespace in the environment resource")
	}
	// identify recipe's template path associated with provided recipe name
	recipe, ok := env.Properties.Recipes[recipeName]
	if ok {
		envMetadata.RecipeLinkType = recipe.LinkType
		envMetadata.RecipeTemplatePath = recipe.TemplatePath
	} else if recipeName != "" {
		return envMetadata, fmt.Errorf("recipe with name %q does not exist in the environment %s", recipeName, environmentID)
	}

	// get the providers metadata to deploy the recipe
	envMetadata.Providers = env.Properties.Providers

	return envMetadata, nil
}
