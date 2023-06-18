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
	"net"
	"os"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/resourcemodel"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	rp_util "github.com/project-radius/radius/pkg/rp/util"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	corerp_dm "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers"

	"github.com/project-radius/radius/pkg/linkrp"
	link_dm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"github.com/go-openapi/jsonpointer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/corerp/backend/deployment github.com/project-radius/radius/pkg/corerp/backend/deployment DeploymentProcessor
type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource v1.DataModelInterface) (renderers.RendererOutput, error)
	Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (rpv1.DeploymentOutput, error)
	Delete(ctx context.Context, id resources.ID, outputResources []rpv1.OutputResource) error
	FetchSecrets(ctx context.Context, resourceData ResourceData) (map[string]any, error)
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, sp dataprovider.DataStorageProvider, secretClient sv.SecretValueClient, k8sClient controller_runtime.Client, k8sClientSet kubernetes.Interface) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, sp: sp, secretClient: secretClient, k8sClient: k8sClient, k8sClientSet: k8sClientSet}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel     model.ApplicationModel
	sp           dataprovider.DataStorageProvider
	secretClient sv.SecretValueClient
	// k8sClient is the Kubernetes controller runtime client.
	k8sClient controller_runtime.Client
	// k8sClientSet is the Kubernetes client.
	k8sClientSet kubernetes.Interface
}

type ResourceData struct {
	ID              resources.ID // resource ID
	Resource        v1.DataModelInterface
	OutputResources []rpv1.OutputResource
	ComputedValues  map[string]any
	SecretValues    map[string]rpv1.SecretValueReference
	AppID           *resources.ID     // Application ID for which the resource is created
	RecipeData      linkrp.RecipeData // Relevant only for links created with recipes to find relevant connections created by that recipe
}

func (dp *deploymentProcessor) Render(ctx context.Context, resourceID resources.ID, resource v1.DataModelInterface) (renderers.RendererOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Rendering resource: %s", resourceID.Name()))
	renderer, err := dp.getResourceRenderer(resourceID)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// get namespace for deploying the resource
	// 1. fetch the resource from the DB and get the application info
	res, err := dp.getResourceDataByID(ctx, resourceID)
	if err != nil {
		// Internal error: this shouldn't happen unless a new supported resource type wasn't added in `getResourceDataByID`
		return renderers.RendererOutput{}, err
	}
	// 2. fetch the application properties from the DB
	app := &corerp_dm.Application{}
	err = rp_util.FetchScopeResource(ctx, dp.sp, res.AppID.String(), app)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	// 3. fetch the environment resource from the db to get the Namespace
	env := &corerp_dm.Environment{}
	err = rp_util.FetchScopeResource(ctx, dp.sp, app.Properties.Environment, env)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Get resources that the resource being deployed has connection with.
	requiredResources, _, err := renderer.GetDependencyIDs(ctx, resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererDependencies, err := dp.fetchDependencies(ctx, requiredResources)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	envOptions, err := dp.getEnvOptions(ctx, env)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	c := app.Properties.Status.Compute
	// Override environment-scope namespace with application-scope kubernetes namespace.
	if c != nil && c.Kind == rpv1.KubernetesComputeKind {
		envOptions.Namespace = c.KubernetesCompute.Namespace
	}

	appOptions, err := dp.getAppOptions(ctx, &app.Properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererOutput, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: rendererDependencies, Environment: envOptions, Application: appOptions})
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Check if the output resources have the corresponding provider supported in Radius
	for _, or := range rendererOutput.Resources {
		if or.ResourceType.Provider == "" {
			return renderers.RendererOutput{}, fmt.Errorf("output resource %q does not have a provider specified", or.LocalID)
		}
		if !dp.appmodel.IsProviderSupported(or.ResourceType.Provider) {
			return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type))
		}
	}

	rendererOutput.RadiusResource = resource

	return rendererOutput, nil
}

func (dp *deploymentProcessor) getResourceRenderer(resourceID resources.ID) (renderers.Renderer, error) {
	radiusResourceModel, err := dp.appmodel.LookupRadiusResourceModel(resourceID.Type())
	if err != nil {
		// Internal error: A resource type with unsupported app model shouldn't have reached here
		return nil, err
	}

	return radiusResourceModel.Renderer, nil
}

func (dp *deploymentProcessor) deployOutputResource(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput, computedValues map[string]any, putOptions *handlers.PutOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	or := putOptions.Resource
	logger.Info(fmt.Sprintf("Deploying output resource: LocalID: %s, resource type: %q\n", or.LocalID, or.ResourceType))

	outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(or.ResourceType)
	if err != nil {
		return err
	}

	// Transform resource before deploying resource.
	if outputResourceModel.ResourceTransformer != nil {
		if err := outputResourceModel.ResourceTransformer(ctx, putOptions); err != nil {
			return err
		}
	}
	properties, err := outputResourceModel.ResourceHandler.Put(ctx, putOptions)
	if err != nil {
		return err
	}

	if or.Identity.ResourceType == nil {
		err = fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler", or.LocalID)
		return err
	}

	putOptions.DependencyProperties[or.LocalID] = properties

	// Copy deployed output resource property values into corresponding expected computed values
	for k, v := range rendererOutput.ComputedValues {
		// A computed value might be a reference to a 'property' returned in preserved properties
		if or.LocalID == v.LocalID && v.PropertyReference != "" {
			computedValues[k] = properties[v.PropertyReference]
			continue
		}

		// A computed value might be a 'pointer' into the deployed resource
		if or.LocalID == v.LocalID && v.JSONPointer != "" {
			pointer, err := jsonpointer.New(v.JSONPointer)
			if err != nil {
				return fmt.Errorf("failed to process JSON Pointer %q for resource: %w", v.JSONPointer, err)
			}

			value, _, err := pointer.Get(or.Resource)
			if err != nil {
				return fmt.Errorf("failed to process JSON Pointer %q for resource: %w", v.JSONPointer, err)
			}
			computedValues[k] = value
		}
	}

	return nil
}

func (dp *deploymentProcessor) Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (rpv1.DeploymentOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Deploy
	logger.Info(fmt.Sprintf("Deploying radius resource: %s", id.Name()))

	// Order output resources in deployment dependency order
	orderedOutputResources, err := rpv1.OrderOutputResources(rendererOutput.Resources)
	if err != nil {
		return rpv1.DeploymentOutput{}, err
	}

	deployedOutputResources := []rpv1.OutputResource{}

	// Values consumed by other Radius resource types through connections
	computedValues := map[string]any{}

	deployedOutputResourceProperties := map[string]map[string]string{}

	for _, outputResource := range orderedOutputResources {
		logger.Info(fmt.Sprintf("Deploying output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))

		err := dp.deployOutputResource(ctx, id, rendererOutput, computedValues, &handlers.PutOptions{Resource: &outputResource, DependencyProperties: deployedOutputResourceProperties})
		if err != nil {
			return rpv1.DeploymentOutput{}, err
		}

		if outputResource.Identity.ResourceType == nil {
			return rpv1.DeploymentOutput{}, fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler", outputResource.LocalID)
		}

		// Build database resource - copy updated properties to Resource field
		outputResource := rpv1.OutputResource{
			LocalID:      outputResource.LocalID,
			ResourceType: outputResource.ResourceType,
			Identity:     outputResource.Identity,
			Status: rpv1.OutputResourceStatus{
				ProvisioningState:        string(v1.ProvisioningStateProvisioned),
				ProvisioningErrorDetails: "",
			},
		}
		deployedOutputResources = append(deployedOutputResources, outputResource)
	}

	// Update static values for connections
	for k, computedValue := range rendererOutput.ComputedValues {
		if computedValue.Value != nil {
			computedValues[k] = computedValue.Value
		}
	}

	// Transform Radius resource with computedValues
	for _, cv := range rendererOutput.ComputedValues {
		if cv.Transformer != nil {
			if err := cv.Transformer(rendererOutput.RadiusResource, computedValues); err != nil {
				return rpv1.DeploymentOutput{}, err
			}
		}
	}

	return rpv1.DeploymentOutput{
		DeployedOutputResources: deployedOutputResources,
		ComputedValues:          computedValues,
		SecretValues:            rendererOutput.SecretValues,
	}, nil
}

func (dp *deploymentProcessor) Delete(ctx context.Context, id resources.ID, deployedOutputResources []rpv1.OutputResource) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Loop over each output resource and delete in reverse dependency order - resource deployed last should be deleted first
	for i := len(deployedOutputResources) - 1; i >= 0; i-- {
		outputResource := deployedOutputResources[i]
		outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
		if err != nil {
			return err
		}

		logger.Info(fmt.Sprintf("Deleting output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))
		err = outputResourceModel.ResourceHandler.Delete(ctx, &handlers.DeleteOptions{Resource: &outputResource})
		if err != nil {
			return err
		}
	}

	return nil
}

// Returns fully qualified radius resource identifier to RendererDependency map
func (dp *deploymentProcessor) fetchDependencies(ctx context.Context, resourceIDs []resources.ID) (map[string]renderers.RendererDependency, error) {
	rendererDependencies := map[string]renderers.RendererDependency{}
	for _, id := range resourceIDs {
		rd, err := dp.getResourceDataByID(ctx, id)
		if err != nil {
			return nil, err
		}

		rendererDependency, err := dp.getRendererDependency(ctx, rd)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch required renderer dependency %q: %w", id.String(), err)
		}

		rendererDependencies[id.String()] = rendererDependency
	}

	return rendererDependencies, nil
}

func (dp *deploymentProcessor) FetchSecrets(ctx context.Context, dependency ResourceData) (map[string]any, error) {
	secretValues := map[string]any{}
	for k, secretReference := range dependency.SecretValues {
		secret, err := dp.fetchSecret(ctx, dependency, secretReference)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch secret %q of dependency resource %q: %w", k, dependency.ID.String(), err)
		}

		if (secretReference.Transformer != resourcemodel.ResourceType{}) {
			outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(secretReference.Transformer)
			if err != nil {
				return nil, err
			} else if outputResourceModel.SecretValueTransformer == nil {
				return nil, fmt.Errorf("could not find a secret transformer for %q", secretReference.Transformer)
			}

			secret, err = outputResourceModel.SecretValueTransformer.Transform(ctx, dependency.ComputedValues, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to transform secret %q of dependency resource %q: %W", k, dependency.ID.String(), err)
			}
		}

		secretValues[k] = secret
	}

	return secretValues, nil
}

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, dependency ResourceData, reference rpv1.SecretValueReference) (any, error) {
	if reference.Value != "" {
		// The secret reference contains the value itself
		return reference.Value, nil
	}

	var match *rpv1.OutputResource
	for _, outputResource := range dependency.OutputResources {
		if outputResource.LocalID == reference.LocalID {
			copy := outputResource
			match = &copy
			break
		}
	}

	if match == nil {
		return nil, fmt.Errorf("cannot find an output resource matching LocalID %q for dependency %q", reference.LocalID, dependency.ID)
	}

	if dp.secretClient == nil {
		return nil, errors.New("no Azure credentials provided to fetch secret")
	}
	return dp.secretClient.FetchSecret(ctx, match.Identity, reference.Action, reference.ValueSelector)
}

// TODO: Revisit to remove the corerp_dm.Environment dependency.
func (dp *deploymentProcessor) getEnvOptions(ctx context.Context, env *corerp_dm.Environment) (renderers.EnvironmentOptions, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	publicEndpointOverride := os.Getenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

	envOpts := renderers.EnvironmentOptions{
		CloudProviders: &env.Properties.Providers,
	}

	// Extract compute info
	switch env.Properties.Compute.Kind {
	case rpv1.KubernetesComputeKind:
		kubeProp := &env.Properties.Compute.KubernetesCompute

		if kubeProp.Namespace == "" {
			return renderers.EnvironmentOptions{}, errors.New("kubernetes' namespace is not specified")
		}
		envOpts.Namespace = kubeProp.Namespace

	default:
		return renderers.EnvironmentOptions{}, fmt.Errorf("%s is unsupported", env.Properties.Compute.Kind)
	}

	// Extract identity info.
	envOpts.Identity = env.Properties.Compute.Identity
	if envOpts.Identity == nil {
		logger.V(ucplog.Debug).Info("environment identity is not specified.")
	}

	// Get Environment KubernetesMetadata Info
	if envExt := corerp_dm.FindExtension(env.Properties.Extensions, corerp_dm.KubernetesMetadata); envExt != nil && envExt.KubernetesMetadata != nil {
		envOpts.KubernetesMetadata = envExt.KubernetesMetadata
	}

	if publicEndpointOverride != "" {
		// Check if publicEndpointOverride contains a scheme,
		// and if so, throw an error to the user
		if strings.HasPrefix(publicEndpointOverride, "http://") || strings.HasPrefix(publicEndpointOverride, "https://") {
			return renderers.EnvironmentOptions{}, errors.New("a URL is not accepted here. Please reinstall Radius with a valid public endpoint using rad install kubernetes --reinstall --public-endpoint-override <your-endpoint>")
		}

		hostname, port, err := net.SplitHostPort(publicEndpointOverride)
		if err != nil {
			// If net.SplitHostPort throws an error, then use
			// publicEndpointOverride as the host
			hostname = publicEndpointOverride
			port = ""
		}

		envOpts.Gateway = renderers.GatewayOptions{
			PublicEndpointOverride: true,
			Hostname:               hostname,
			Port:                   port,
		}

		return envOpts, nil
	}

	if dp.k8sClient != nil {
		// Find the public endpoint of the cluster (External IP or hostname of the contour-envoy service)
		var services corev1.ServiceList
		err := dp.k8sClient.List(ctx, &services, &controller_runtime.ListOptions{Namespace: "radius-system"})
		if err != nil {
			return renderers.EnvironmentOptions{}, fmt.Errorf("failed to look up Services: %w", err)
		}

		for _, service := range services.Items {
			if service.Name == "contour-envoy" {
				for _, in := range service.Status.LoadBalancer.Ingress {
					envOpts.Gateway = renderers.GatewayOptions{
						PublicEndpointOverride: false,
						Hostname:               in.Hostname,
						ExternalIP:             in.IP,
					}
					return envOpts, nil
				}
			}
		}
	}

	return envOpts, nil
}

// getAppOptions: Populates and Returns ApplicationOptions.
func (dp *deploymentProcessor) getAppOptions(ctx context.Context, appProp *corerp_dm.ApplicationProperties) (renderers.ApplicationOptions, error) {
	appOpts := renderers.ApplicationOptions{}

	// Get Application KubernetesMetadata Info
	if ext := corerp_dm.FindExtension(appProp.Extensions, corerp_dm.KubernetesMetadata); ext != nil && ext.KubernetesMetadata != nil {
		appOpts.KubernetesMetadata = ext.KubernetesMetadata
	}

	return appOpts, nil
}

// getResourceDataByID fetches resource for the provided id from the data store
func (dp *deploymentProcessor) getResourceDataByID(ctx context.Context, resourceID resources.ID) (ResourceData, error) {
	errMsg := "failed to fetch the resource %q. Err: %w"
	sc, err := dp.sp.GetStorageClient(ctx, resourceID.Type())
	if err != nil {
		return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
	}

	resource, err := sc.Get(ctx, resourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return ResourceData{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("resource %q does not exist", resourceID.String()))
		}
		return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
	}

	resourceType := strings.ToLower(resourceID.Type())
	switch resourceType {
	case strings.ToLower(corerp_dm.ContainerResourceType):
		obj := &corerp_dm.ContainerResource{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, linkrp.RecipeData{})
	case strings.ToLower(corerp_dm.GatewayResourceType):
		obj := &corerp_dm.Gateway{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, linkrp.RecipeData{})
	case strings.ToLower(corerp_dm.VolumeResourceType):
		obj := &corerp_dm.VolumeResource{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, linkrp.RecipeData{})
	case strings.ToLower(corerp_dm.HTTPRouteResourceType):
		obj := &corerp_dm.HTTPRoute{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, linkrp.RecipeData{})
	case strings.ToLower(corerp_dm.SecretStoreResourceType):
		obj := &corerp_dm.SecretStore{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, linkrp.RecipeData{})
	case strings.ToLower(linkrp.MongoDatabasesResourceType):
		obj := &link_dm.MongoDatabase{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		obj := &link_dm.SqlDatabase{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	case strings.ToLower(linkrp.RedisCachesResourceType):
		obj := &link_dm.RedisCache{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		obj := &link_dm.RabbitMQMessageQueue{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	case strings.ToLower(linkrp.ExtendersResourceType):
		obj := &link_dm.Extender{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, linkrp.RecipeData{})
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		obj := &link_dm.DaprStateStore{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		obj := &link_dm.DaprSecretStore{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		obj := &link_dm.DaprPubSubBroker{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues, obj.RecipeData)
	default:
		return ResourceData{}, fmt.Errorf("unsupported resource type: %q for resource ID: %q", resourceType, resourceID.String())
	}
}

func (dp *deploymentProcessor) buildResourceDependency(resourceID resources.ID, applicationID string, resource v1.DataModelInterface, outputResources []rpv1.OutputResource, computedValues map[string]any, secretValues map[string]rpv1.SecretValueReference, recipeData linkrp.RecipeData) (ResourceData, error) {
	var appID *resources.ID
	if applicationID != "" {
		parsedID, err := resources.ParseResource(applicationID)
		if err != nil {
			return ResourceData{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("application ID %q for the resource %q is not a valid id. Error: %s", applicationID, resourceID.String(), err.Error()))
		}
		appID = &parsedID
	} else if strings.EqualFold(resourceID.ProviderNamespace(), resources.LinkRPNamespace) {
		// Application id is optional for link resource types
		appID = nil
	} else {
		return ResourceData{}, fmt.Errorf("missing required application id for the resource %q", resourceID.String())
	}

	return ResourceData{
		ID:              resourceID,
		Resource:        resource,
		OutputResources: outputResources,
		ComputedValues:  computedValues,
		SecretValues:    secretValues,
		AppID:           appID,
		RecipeData:      recipeData,
	}, nil
}

func (dp *deploymentProcessor) getRendererDependency(ctx context.Context, dependency ResourceData) (renderers.RendererDependency, error) {
	// Get dependent resource identity
	outputResourceIdentity := map[string]resourcemodel.ResourceIdentity{}
	for _, outputResource := range dependency.OutputResources {
		outputResourceIdentity[outputResource.LocalID] = outputResource.Identity
	}

	// Get  dependent resource computedValues
	computedValues := map[string]any{}
	for k, v := range dependency.ComputedValues {
		computedValues[k] = v
	}

	// Get  dependent resource secretValues
	secretValues, err := dp.FetchSecrets(ctx, dependency)
	if err != nil {
		return renderers.RendererDependency{}, err
	}

	// Make dependent resource secretValues as part of computedValues
	for k, v := range secretValues {
		computedValues[k] = v
	}

	// Now build the renderer dependecy out of these collected dependencies
	rendererDependency := renderers.RendererDependency{
		ResourceID:      dependency.ID,
		Resource:        dependency.Resource,
		ComputedValues:  computedValues,
		OutputResources: outputResourceIdentity,
	}

	return rendererDependency, nil
}
