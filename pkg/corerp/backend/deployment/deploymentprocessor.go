// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	connector_dm "github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/project-radius/radius/pkg/connectorrp/renderers/daprinvokehttproutes"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/daprpubsubbrokers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/daprsecretstores"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/daprstatestores"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/extenders"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/rabbitmqmessagequeues"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/rediscaches"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/sqldatabases"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/corerp/backend/deployment github.com/project-radius/radius/pkg/corerp/backend/deployment DeploymentProcessor
type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error)
	Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (rp.DeploymentOutput, error)
	Delete(ctx context.Context, id resources.ID, outputResources []outputresource.OutputResource) error
	FetchSecrets(ctx context.Context, resourceData ResourceData) (map[string]interface{}, error)
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, sp dataprovider.DataStorageProvider, secretClient renderers.SecretValueClient, k8sClient controller_runtime.Client, k8sClientSet kubernetes.Interface) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, sp: sp, secretClient: secretClient, k8sClient: k8sClient, k8sClientSet: k8sClientSet}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel     model.ApplicationModel
	sp           dataprovider.DataStorageProvider
	secretClient renderers.SecretValueClient
	// k8sClient is the Kubernetes controller runtime client.
	k8sClient controller_runtime.Client
	// k8sClientSet is the Kubernetes client.
	k8sClientSet kubernetes.Interface
}

type ResourceData struct {
	ID              resources.ID // resource ID
	Resource        conv.DataModelInterface
	OutputResources []outputresource.OutputResource
	ComputedValues  map[string]interface{}
	SecretValues    map[string]rp.SecretValueReference
	AppID           resources.ID // Application ID for which the resource is created
}

func (dp *deploymentProcessor) Render(ctx context.Context, resourceID resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error) {
	logger := radlogger.GetLogger(ctx)
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
	// 2. fetch the application resource from the DB to get the environment info
	environment, err := dp.getEnvironmentFromApplication(ctx, res.AppID, resourceID.String())
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	// 3. fetch the environment resource from the db to get the Namespace
	namespace, err := dp.getEnvironmentNamespace(ctx, environment, resourceID.String())
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

	envOptions, err := dp.getEnvOptions(ctx, namespace)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererOutput, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: rendererDependencies, Environment: envOptions})
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Check if the output resources have the corresponding provider supported in Radius
	for _, or := range rendererOutput.Resources {
		if or.ResourceType.Provider == "" {
			return renderers.RendererOutput{}, fmt.Errorf("output resource %q does not have a provider specified", or.LocalID)
		}
		if !dp.appmodel.IsProviderSupported(or.ResourceType.Provider) {
			return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type))
		}
	}

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

func (dp *deploymentProcessor) deployOutputResource(ctx context.Context, id resources.ID, outputResource outputresource.OutputResource, rendererOutput renderers.RendererOutput) (resourceIdentity resourcemodel.ResourceIdentity, computedValues map[string]interface{}, err error) {
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Deploying output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))

	outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	resourceIdentity, err = outputResourceModel.ResourceHandler.GetResourceIdentity(ctx, outputResource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = outputResourceModel.ResourceHandler.Put(ctx, &outputResource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	properties, err := outputResourceModel.ResourceHandler.GetResourceNativeIdentityKeyProperties(ctx, outputResource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Values consumed by other Radius resource types through connections
	computedValues = map[string]interface{}{}

	// Copy deployed output resource property values into corresponding expected computed values
	for k, v := range rendererOutput.ComputedValues {
		// A computed value might be a reference to a 'property' returned in preserved properties
		if outputResource.LocalID == v.LocalID && v.PropertyReference != "" {
			computedValues[k] = properties[v.PropertyReference]
			continue
		}

		// A computed value might be a 'pointer' into the deployed resource
		if outputResource.LocalID == v.LocalID && v.JSONPointer != "" {
			pointer, err := jsonpointer.New(v.JSONPointer)
			if err != nil {
				err = fmt.Errorf("failed to process JSON Pointer %q for resource: %w", v.JSONPointer, err)
				return resourcemodel.ResourceIdentity{}, nil, err
			}

			value, _, err := pointer.Get(outputResource.Resource)
			if err != nil {
				err = fmt.Errorf("failed to process JSON Pointer %q for resource: %w", v.JSONPointer, err)
				return resourcemodel.ResourceIdentity{}, nil, err
			}
			computedValues[k] = value
		}
	}

	return resourceIdentity, computedValues, nil
}

func (dp *deploymentProcessor) Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (rp.DeploymentOutput, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, id.String())

	// Deploy
	logger.Info(fmt.Sprintf("Deploying radius resource: %s", id.Name()))

	// Order output resources in deployment dependency order
	orderedOutputResources, err := outputresource.OrderOutputResources(rendererOutput.Resources)
	if err != nil {
		return rp.DeploymentOutput{}, err
	}

	deployedOutputResources := []outputresource.OutputResource{}

	// Values consumed by other Radius resource types through connections
	computedValues := map[string]interface{}{}

	for _, outputResource := range orderedOutputResources {
		logger.Info(fmt.Sprintf("Deploying output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))

		resourceIdentity, deployedComputedValues, err := dp.deployOutputResource(ctx, id, outputResource, rendererOutput)
		if err != nil {
			return rp.DeploymentOutput{}, err
		}

		if (resourceIdentity != resourcemodel.ResourceIdentity{}) {
			outputResource.Identity = resourceIdentity
		}

		if outputResource.Identity.ResourceType == nil {
			return rp.DeploymentOutput{}, fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler", outputResource.LocalID)
		}

		// Build database resource - copy updated properties to Resource field
		outputResource := outputresource.OutputResource{
			LocalID:      outputResource.LocalID,
			ResourceType: outputResource.ResourceType,
			Identity:     outputResource.Identity,
			Status: outputresource.OutputResourceStatus{
				ProvisioningState:        string(v1.ProvisioningStateProvisioned),
				ProvisioningErrorDetails: "",
			},
		}
		deployedOutputResources = append(deployedOutputResources, outputResource)
		computedValues = deployedComputedValues
	}

	// Update static values for connections
	for k, computedValue := range rendererOutput.ComputedValues {
		if computedValue.Value != nil {
			computedValues[k] = computedValue.Value
		}
	}

	return rp.DeploymentOutput{
		DeployedOutputResources: deployedOutputResources,
		ComputedValues:          computedValues,
		SecretValues:            rendererOutput.SecretValues,
	}, nil
}

func (dp *deploymentProcessor) Delete(ctx context.Context, id resources.ID, deployedOutputResources []outputresource.OutputResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, id)

	// Loop over each output resource and delete in reverse dependency order - resource deployed last should be deleted first
	for i := len(deployedOutputResources) - 1; i >= 0; i-- {
		outputResource := deployedOutputResources[i]
		outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(outputResource.ResourceType)
		if err != nil {
			return err
		}

		logger.Info(fmt.Sprintf("Deleting output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))
		err = outputResourceModel.ResourceHandler.Delete(ctx, outputResource)
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

func (dp *deploymentProcessor) FetchSecrets(ctx context.Context, dependency ResourceData) (map[string]interface{}, error) {
	computedValues := map[string]interface{}{}
	for k, v := range dependency.ComputedValues {
		computedValues[k] = v
	}

	rendererDependency := renderers.RendererDependency{
		ResourceID:     dependency.ID,
		ComputedValues: computedValues,
	}

	secretValues := map[string]interface{}{}
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

			secret, err = outputResourceModel.SecretValueTransformer.Transform(ctx, rendererDependency, secret)
			if err != nil {
				return nil, fmt.Errorf("failed to transform secret %q of dependency resource %q: %W", k, dependency.ID.String(), err)
			}
		}

		secretValues[k] = secret
	}

	return secretValues, nil
}

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, dependency ResourceData, reference rp.SecretValueReference) (interface{}, error) {
	if reference.Value != "" {
		// The secret reference contains the value itself
		return reference.Value, nil
	}

	var match *outputresource.OutputResource
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

func (dp *deploymentProcessor) getEnvOptions(ctx context.Context, namespace string) (renderers.EnvironmentOptions, error) {
	publicEndpointOverride := os.Getenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")
	if publicEndpointOverride != "" {
		// Check if publicEndpointOverride contains a scheme,
		// and if so, throw an error to the user
		if strings.HasPrefix(publicEndpointOverride, "http://") || strings.HasPrefix(publicEndpointOverride, "https://") {
			return renderers.EnvironmentOptions{}, fmt.Errorf("a URL is not accepted here. Please reinstall Radius with a valid public endpoint using rad install kubernetes --reinstall --public-endpoint-override <your-endpoint>")
		}

		hostname, port, err := net.SplitHostPort(publicEndpointOverride)
		if err != nil {
			// If net.SplitHostPort throws an error, then use
			// publicEndpointOverride as the host
			hostname = publicEndpointOverride
			port = ""
		}

		return renderers.EnvironmentOptions{
			Gateway: renderers.GatewayOptions{
				PublicEndpointOverride: true,
				Hostname:               hostname,
				Port:                   port,
			},
			Namespace: namespace,
		}, nil
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
					return renderers.EnvironmentOptions{
						Gateway: renderers.GatewayOptions{
							PublicEndpointOverride: false,
							Hostname:               in.Hostname,
							ExternalIP:             in.IP,
						},
						Namespace: namespace,
					}, nil
				}
			}
		}
	}

	return renderers.EnvironmentOptions{Namespace: namespace}, nil
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
			return ResourceData{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("resource %q does not exist", resourceID.String()))
		}
		return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
	}

	resourceType := strings.ToLower(resourceID.Type())
	switch resourceType {
	case strings.ToLower(container.ResourceType):
		obj := &datamodel.ContainerResource{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(gateway.ResourceType):
		obj := &datamodel.Gateway{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(httproute.ResourceType):
		obj := &datamodel.HTTPRoute{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(mongodatabases.ResourceType):
		obj := &connector_dm.MongoDatabase{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(sqldatabases.ResourceType):
		obj := &connector_dm.SqlDatabase{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(rediscaches.ResourceType):
		obj := &connector_dm.RedisCache{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(rabbitmqmessagequeues.ResourceType):
		obj := &connector_dm.RabbitMQMessageQueue{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(extenders.ResourceType):
		obj := &connector_dm.Extender{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(daprstatestores.ResourceType):
		obj := &connector_dm.DaprStateStore{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(daprsecretstores.ResourceType):
		obj := &connector_dm.DaprSecretStore{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(daprpubsubbrokers.ResourceType):
		obj := &connector_dm.DaprPubSubBroker{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	case strings.ToLower(daprinvokehttproutes.ResourceType):
		obj := &connector_dm.DaprInvokeHttpRoute{}
		if err = resource.As(obj); err != nil {
			return ResourceData{}, fmt.Errorf(errMsg, resourceID.String(), err)
		}
		return dp.buildResourceDependency(resourceID, obj.Properties.Application, obj, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues)
	default:
		return ResourceData{}, fmt.Errorf("unsupported resource type: %q for resource ID: %q", resourceType, resourceID.String())
	}
}

func (dp *deploymentProcessor) buildResourceDependency(resourceID resources.ID, applicationID string, resource conv.DataModelInterface, outputResources []outputresource.OutputResource, computedValues map[string]interface{}, secretValues map[string]rp.SecretValueReference) (ResourceData, error) {
	var appID resources.ID
	if applicationID != "" {
		parsedID, err := resources.Parse(applicationID)
		if err != nil {
			return ResourceData{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("application ID %q for the resource %q is not a valid id. Error: %s", applicationID, resourceID.String(), err.Error()))
		}
		appID = parsedID
	} else if strings.EqualFold(resourceID.ProviderNamespace(), resources.ConnectorRPNamespace) {
		// Application id is optional for connector resource types
		appID = resources.ID{}
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
	}, nil
}

func (dp *deploymentProcessor) getRendererDependency(ctx context.Context, dependency ResourceData) (renderers.RendererDependency, error) {
	// Get dependent resource identity
	outputResourceIdentity := map[string]resourcemodel.ResourceIdentity{}
	for _, outputResource := range dependency.OutputResources {
		outputResourceIdentity[outputResource.LocalID] = outputResource.Identity
	}

	// Get  dependent resource computedValues
	computedValues := map[string]interface{}{}
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
		ComputedValues:  computedValues,
		OutputResources: outputResourceIdentity,
	}

	return rendererDependency, nil
}

// getEnvironmentFromApplication returns environment id linked to the application fetched from the db
func (dp *deploymentProcessor) getEnvironmentFromApplication(ctx context.Context, appID resources.ID, resourceID string) (string, error) {
	errMsg := "failed to fetch the application %q for the resource %q. Err: %w"

	appIDType := appID.Type()
	app := &datamodel.Application{}
	if !strings.EqualFold(appIDType, app.ResourceTypeName()) {
		return "", conv.NewClientErrInvalidRequest(fmt.Sprintf("linked application ID %q for resource %q has invalid application resource type.", appID.String(), resourceID))
	}

	sc, err := dp.sp.GetStorageClient(ctx, appIDType)
	if err != nil {
		return "", fmt.Errorf(errMsg, appID.String(), resourceID, err)
	}

	res, err := sc.Get(ctx, appID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return "", conv.NewClientErrInvalidRequest(fmt.Sprintf("linked application %q for resource %q does not exist", appID.String(), resourceID))
		}
		return "", fmt.Errorf(errMsg, appID.String(), resourceID, err)
	}
	err = res.As(app)
	if err != nil {
		return "", fmt.Errorf(errMsg, appID.String(), resourceID, err)
	}

	return app.Properties.Environment, nil
}

// getEnvironmentNamespace fetches the environment resource from the db for getting the namespace to deploy the resources
func (dp *deploymentProcessor) getEnvironmentNamespace(ctx context.Context, environmentID, resourceID string) (string, error) {
	envId, err := resources.Parse(environmentID)
	if err != nil {
		return "", conv.NewClientErrInvalidRequest(fmt.Sprintf("environment id %q linked to the application for resource %q is not a valid id. Error: %s", environmentID, resourceID, err.Error()))
	}

	env := &datamodel.Environment{}
	if !strings.EqualFold(envId.Type(), env.ResourceTypeName()) {
		return "", conv.NewClientErrInvalidRequest(fmt.Sprintf("environment id %q linked to the application for resource %q is not a valid environment type. Error: %s", envId.Type(), resourceID, err.Error()))
	}

	errMsg := "failed to fetch the environment %q for the resource %q. Error: %w"
	sc, err := dp.sp.GetStorageClient(ctx, envId.Type())
	if err != nil {
		return "", fmt.Errorf(errMsg, environmentID, resourceID, err)
	}
	res, err := sc.Get(ctx, envId.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return "", conv.NewClientErrInvalidRequest(fmt.Sprintf("linked environment %q for resource %q does not exist", environmentID, resourceID))
		}
		return "", fmt.Errorf(errMsg, environmentID, resourceID, err)
	}
	err = res.As(env)
	if err != nil {
		return "", fmt.Errorf(errMsg, environmentID, resourceID, err)
	}

	if env.Properties != (datamodel.EnvironmentProperties{}) && env.Properties.Compute != (datamodel.EnvironmentCompute{}) && env.Properties.Compute.KubernetesCompute != (datamodel.KubernetesComputeProperties{}) {
		return env.Properties.Compute.KubernetesCompute.Namespace, nil
	} else {
		return "", fmt.Errorf("cannot find namespace in the environment resource")
	}
}
