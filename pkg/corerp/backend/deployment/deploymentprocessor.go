// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-openapi/jsonpointer"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/corerp/backend/deployment github.com/project-radius/radius/pkg/corerp/backend/deployment DeploymentProcessor
type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error)
	Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (DeploymentOutput, error)
	Delete(ctx context.Context, id resources.ID, resource conv.DataModelInterface) error
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, sp dataprovider.DataStorageProvider, secretClient renderers.SecretValueClient, k8s client.Client) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, sp: sp, secretClient: secretClient, k8s: k8s}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel     model.ApplicationModel
	sp           dataprovider.DataStorageProvider
	secretClient renderers.SecretValueClient
	k8s          client.Client
}

type DeploymentOutput struct {
	DeployedOutputResources []outputresource.OutputResource
	ComputedValues          map[string]interface{}
	SecretValues            map[string]rp.SecretValueReference
}

type ResourceDependency struct {
	ID              resources.ID
	OutputResources []map[string]outputresource.OutputResource
	ComputedValues  map[string]interface{}
	SecretValues    map[string]rp.SecretValueReference
}

func (dp *deploymentProcessor) Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error) {
	resourceID := id.Truncate()
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Rendering resource: %s", resourceID.Name()))
	renderer, err := dp.getResourceRenderer(resourceID)
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

	envOptions, err := dp.getEnvOptions(ctx)
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
			err = fmt.Errorf("output resource %q does not have a provider specified", or.LocalID)
			return renderers.RendererOutput{}, err
		}
		if !dp.appmodel.IsProviderSupported(or.ResourceType.Provider) {
			err := fmt.Errorf("provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type)
			return renderers.RendererOutput{}, err
		}
	}

	return rendererOutput, nil
}

func (dp *deploymentProcessor) getResourceRenderer(resourceID resources.ID) (renderers.Renderer, error) {
	radiusResourceModel, err := dp.appmodel.LookupRadiusResourceModel(resourceID.Type())
	if err != nil {
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

	err = outputResourceModel.ResourceHandler.Put(ctx, outputResource)
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

func (dp *deploymentProcessor) Deploy(ctx context.Context, id resources.ID, rendererOutput renderers.RendererOutput) (DeploymentOutput, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, id.String())

	// Deploy
	logger.Info(fmt.Sprintf("Deploying radius resource: %s", id.Name()))

	// Order output resources in deployment dependency order
	orderedOutputResources, err := outputresource.OrderOutputResources(rendererOutput.Resources)
	if err != nil {
		return DeploymentOutput{}, err
	}

	deployedOutputResources := []outputresource.OutputResource{}

	// Values consumed by other Radius resource types through connections
	computedValues := map[string]interface{}{}

	for _, outputResource := range orderedOutputResources {
		logger.Info(fmt.Sprintf("Deploying output resource: LocalID: %s, resource type: %q\n", outputResource.LocalID, outputResource.ResourceType))

		resourceIdentity, deployedComputedValues, err := dp.deployOutputResource(ctx, id, outputResource, rendererOutput)
		if err != nil {
			return DeploymentOutput{}, err
		}

		if (resourceIdentity != resourcemodel.ResourceIdentity{}) {
			outputResource.Identity = resourceIdentity
		}

		if outputResource.Identity.ResourceType == nil {
			err = fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler", outputResource.LocalID)
			return DeploymentOutput{}, err
		}

		// Build database resource - copy updated properties to Resource field
		outputResource := outputresource.OutputResource{
			LocalID:      outputResource.LocalID,
			ResourceType: outputResource.ResourceType,
			Identity:     outputResource.Identity,
			Status: outputresource.OutputResourceStatus{
				ProvisioningState:        db.Provisioned,
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

	return DeploymentOutput{
		DeployedOutputResources: deployedOutputResources,
		ComputedValues:          computedValues,
		SecretValues:            rendererOutput.SecretValues,
	}, nil
}

func (dp *deploymentProcessor) Delete(ctx context.Context, id resources.ID, resource conv.DataModelInterface) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldOperationID, id)
	resourceID := id.Truncate()

	// Loop over each output resource and delete in reverse dependency order - resource deployed last should be deleted first
	resourceDependency, err := dp.getRequiredResourceDependenciesFromResource(ctx, resourceID, resource)
	if err != nil {
		return err
	}

	deployedOutputResources := resourceDependency.OutputResources
	for i := len(deployedOutputResources) - 1; i >= 0; i-- {
		outputResource := deployedOutputResources[i]
		for _, v := range outputResource {
			outputResourceModel, err := dp.appmodel.LookupOutputResourceModel(v.ResourceType)
			if err != nil {
				return err
			}

			logger.Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %q\n", v.Identity, v.LocalID, v.ResourceType))
			err = outputResourceModel.ResourceHandler.Delete(ctx, v)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Returns fully qualified radius resource identifier to RendererDependency map
func (dp *deploymentProcessor) fetchDependencies(ctx context.Context, resourceIDs []resources.ID) (map[string]renderers.RendererDependency, error) {
	rendererDependencies := map[string]renderers.RendererDependency{}
	for _, id := range resourceIDs {
		rd, err := dp.getRequiredResourceDependenciesFromID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch required resource dependencies %q: %w", id.String(), err)
		}

		rendererDependency, err := dp.getRendererDependency(ctx, rd)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch required renderer dependency %q: %w", id.String(), err)
		}

		rendererDependencies[id.String()] = rendererDependency
	}

	return rendererDependencies, nil
}

func (dp *deploymentProcessor) fetchSecretsInternal(ctx context.Context, dependency ResourceDependency) (map[string]interface{}, error) {
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

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, dependency ResourceDependency, reference rp.SecretValueReference) (interface{}, error) {
	if reference.Value != "" {
		// The secret reference contains the value itself
		return reference.Value, nil
	}

	var match *outputresource.OutputResource
	for _, outputResource := range dependency.OutputResources {
		for k, v := range outputResource {
			if k == reference.LocalID {
				copy := v
				match = &copy
				break
			}
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

func (dp *deploymentProcessor) getEnvOptions(ctx context.Context) (renderers.EnvironmentOptions, error) {
	if dp.k8s != nil {
		// If the public endpoint override is specified (Local Dev scenario), then use it.
		publicEndpoint := os.Getenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")
		if publicEndpoint != "" {
			return renderers.EnvironmentOptions{
				Gateway: renderers.GatewayOptions{
					PublicEndpointOverride: true,
					PublicIP:               publicEndpoint,
				},
			}, nil
		}

		// Find the public IP of the cluster (External IP of the contour-envoy service)
		var services corev1.ServiceList
		err := dp.k8s.List(ctx, &services, &client.ListOptions{Namespace: "radius-system"})
		if err != nil {
			return renderers.EnvironmentOptions{}, fmt.Errorf("failed to look up Services: %w", err)
		}

		for _, service := range services.Items {
			if service.Name == "contour-envoy" {
				for _, in := range service.Status.LoadBalancer.Ingress {
					return renderers.EnvironmentOptions{
						Gateway: renderers.GatewayOptions{
							PublicEndpointOverride: false,
							PublicIP:               in.IP,
						},
					}, nil
				}
			}
		}
	}

	return renderers.EnvironmentOptions{}, nil
}

// getRequiredResourceDependenciesFromID is to get the resource dependencies.
func (dp *deploymentProcessor) getRequiredResourceDependenciesFromID(ctx context.Context, resourceID resources.ID) (ResourceDependency, error) {
	var res *store.Object
	var err error
	var sc store.StorageClient
	sc, err = dp.sp.GetStorageClient(ctx, resourceID.Type())
	if err != nil {
		return ResourceDependency{}, err
	}

	resourceType := resourceID.Type()
	switch resourceType {
	case container.ResourceType:
		obj := &datamodel.ContainerResource{}
		if res, err = sc.Get(ctx, resourceID.String()); err == nil {
			if err = res.As(obj); err == nil {
				return dp.buildResourceDependency(resourceID, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues), nil
			}
		}
	case gateway.ResourceType:
		obj := &datamodel.Gateway{}
		if res, err = sc.Get(ctx, resourceID.String()); err == nil {
			if err = res.As(obj); err == nil {
				return dp.buildResourceDependency(resourceID, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues), nil
			}
		}
	case httproute.ResourceType:
		obj := &datamodel.HTTPRoute{}
		if res, err = sc.Get(ctx, resourceID.String()); err == nil {
			if err = res.As(obj); err == nil {
				return dp.buildResourceDependency(resourceID, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues), nil
			}
		}
	default:
		err = fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, resourceID.String())
	}

	return ResourceDependency{}, err
}

// getRequiredResourceDependenciesFromResource is to get the resource dependencies.
func (dp *deploymentProcessor) getRequiredResourceDependenciesFromResource(ctx context.Context, resourceID resources.ID, resource conv.DataModelInterface) (ResourceDependency, error) {
	var err error
	resourceType := resource.ResourceTypeName()
	switch resourceType {
	case container.ResourceType:
		obj := resource.(datamodel.ContainerResource)
		return dp.buildResourceDependency(resourceID, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues), nil
	case gateway.ResourceType:
		obj := &datamodel.Gateway{}
		return dp.buildResourceDependency(resourceID, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues), nil
	case httproute.ResourceType:
		obj := &datamodel.HTTPRoute{}
		return dp.buildResourceDependency(resourceID, obj.Properties.Status.OutputResources, obj.ComputedValues, obj.SecretValues), nil
	default:
		err = fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, resourceID.String())
	}

	return ResourceDependency{}, err
}

func (dp *deploymentProcessor) buildResourceDependency(resourceID resources.ID, outputResources []outputresource.OutputResource, computedValues map[string]interface{}, secretValues map[string]rp.SecretValueReference) ResourceDependency {
	lst := []map[string]outputresource.OutputResource{}
	for _, r := range outputResources {
		mp := map[string]outputresource.OutputResource{}
		mp[r.LocalID] = r
		lst = append(lst, mp)
	}

	return ResourceDependency{
		ID:              resourceID,
		OutputResources: lst,
		ComputedValues:  computedValues,
		SecretValues:    secretValues,
	}
}

func (dp *deploymentProcessor) getRendererDependency(ctx context.Context, dependency ResourceDependency) (renderers.RendererDependency, error) {
	// Get dependent resource identity
	outputResourceIdentity := map[string]resourcemodel.ResourceIdentity{}
	for _, outputResource := range dependency.OutputResources {
		for k, v := range outputResource {
			outputResourceIdentity[k] = v.Identity
		}
	}

	// Get  dependent resource computedValues
	computedValues := map[string]interface{}{}
	for k, v := range dependency.ComputedValues {
		computedValues[k] = v
	}

	// Get  dependent resource secretValues
	secretValues, err := dp.fetchSecretsInternal(ctx, dependency)
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
