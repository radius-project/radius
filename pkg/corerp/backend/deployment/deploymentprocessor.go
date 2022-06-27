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
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/model"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=./mock_deploymentprocessor.go -package=deployment -self_package github.com/project-radius/radius/pkg/radrp/backend/deployment github.com/project-radius/radius/pkg/radrp/backend/deployment DeploymentProcessor

type DeploymentProcessor interface {
	Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error)
	Deploy(ctx context.Context, operationID resources.ID, rendererOutput renderers.RendererOutput) (DeploymentOutput, error)
}

func NewDeploymentProcessor(appmodel model.ApplicationModel, db store.StorageClient, secretClient renderers.SecretValueClient, k8s client.Client) DeploymentProcessor {
	return &deploymentProcessor{appmodel: appmodel, db: db, secretClient: secretClient, k8s: k8s}
}

var _ DeploymentProcessor = (*deploymentProcessor)(nil)

type deploymentProcessor struct {
	appmodel     model.ApplicationModel
	db           store.StorageClient
	secretClient renderers.SecretValueClient
	k8s          client.Client
}

type DeploymentOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]interface{}
	SecretValues   map[string]renderers.SecretValueReference
}

func (dp *deploymentProcessor) Render(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (renderers.RendererOutput, error) {
	resourceID := id.Truncate()
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Rendering resource: %s, application: %s", resourceID.Name(), resourceID.FindScope(resource.ResourceTypeName())))
	renderer, err := dp.getResourceRenderer(resourceID)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// Get resources that the resource being deployed has connection with.
	radiusDependencyResourceIDs, _, err := renderer.GetDependencyIDs(ctx, resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	rendererDependencies, err := dp.fetchDependencies(ctx, radiusDependencyResourceIDs)
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
			err := fmt.Errorf("Provider %s is not configured. Cannot support resource type %s", or.ResourceType.Provider, or.ResourceType.Type)
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
	logger.Info(fmt.Sprintf("Deploying output resource: %v, LocalID: %s, resource type: %q\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType))

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

	properties, err := outputResourceModel.ResourceHandler.GetResourceNativeIdentityKey(ctx, outputResource)
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
		logger.Info(fmt.Sprintf("Deploying output resource: %v, LocalID: %s, resource type: %q\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType))

		resourceIdentity, deployedComputedValues, err := dp.deployOutputResource(ctx, id, outputResource, rendererOutput)
		if err != nil {
			return DeploymentOutput{}, err
		}

		outputResource.Identity = resourceIdentity
		if outputResource.Identity.ResourceType == nil {
			err = fmt.Errorf("output resource %q does not have an identity. This is a bug in the handler", outputResource.LocalID)
			return DeploymentOutput{}, err
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
		Resources:      deployedOutputResources,
		ComputedValues: computedValues,
		SecretValues:   rendererOutput.SecretValues,
	}, nil
}

// Returns fully qualified radius resource identifier to RendererDependency map
func (dp *deploymentProcessor) fetchDependencies(ctx context.Context, dependencyResourceIDs []resources.ID) (map[string]renderers.RendererDependency, error) {
	rendererDependencies := map[string]renderers.RendererDependency{}
	for _, dependencyResourceID := range dependencyResourceIDs {
		// Fetch resource from db
		// TODO: type switch
		// get the resource type from dependencyResourceID parsing
		dbDependencyResource := &datamodel.ContainerResource{}
		_, err := dp.getResource(ctx, dependencyResourceID.String(), dbDependencyResource)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch dependency resource %q: %w", dependencyResourceID, err)
		}

		dependencyOutputResources := map[string]resourcemodel.ResourceIdentity{}
		for _, outputResource := range dbDependencyResource.Status.OutputResources {
			dependencyOutputResources[outputResource.LocalID] = outputResource.Identity
		}

		// We already have all of the computed values (stored in our database), but we need to look secrets
		// (not stored in our database) and add them to the computed values.
		computedValues := map[string]interface{}{}
		for k, v := range dbDependencyResource.ComputedValues {
			computedValues[k] = v
		}

		secretValues, err := dp.FetchSecrets(ctx, dependencyResourceID, dbDependencyResource)
		if err != nil {
			return nil, err
		}

		for k, v := range secretValues {
			computedValues[k] = v
		}

		rendererDependency := renderers.RendererDependency{
			ResourceID:      dependencyResourceID,
			Definition:      dbDependencyResource.Definition,
			ComputedValues:  computedValues,
			OutputResources: dependencyOutputResources,
		}

		rendererDependencies[dependencyResourceID.ID] = rendererDependency
	}

	return rendererDependencies, nil
}

func (dp *deploymentProcessor) FetchSecrets(ctx context.Context, id resources.ID, resource conv.DataModelInterface) (map[string]interface{}, error) {
	// We already have all of the computed values (stored in our database), but we need to look secrets
	// (not stored in our database) and add them to the computed values.
	computedValues := map[string]interface{}{}
	for k, v := range resource.ComputedValues {
		computedValues[k] = v
	}

	rendererDependency := renderers.RendererDependency{
		ResourceID:     id,
		Definition:     resource.Definition,
		ComputedValues: computedValues,
	}

	secretValues := map[string]interface{}{}
	for k, secretReference := range resource.SecretValues {
		secret, err := dp.fetchSecret(ctx, resource, secretReference)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch secret %q of dependency resource %q: %w", k, id.ID, err)
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
				return nil, fmt.Errorf("failed to transform secret %q of dependency resource %q: %W", k, id.ID, err)
			}
		}

		secretValues[k] = secret
	}

	return secretValues, nil
}

func (dp *deploymentProcessor) fetchSecret(ctx context.Context, dependency db.RadiusResource, reference db.SecretValueReference) (interface{}, error) {
	if reference.Value != nil {
		// The secret reference contains the value itself
		return *reference.Value, nil
	}

	var match *db.OutputResource
	for _, outputResource := range dependency.Status.OutputResources {
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

// getResource is the helper to get the resource via storage client.
func (dp *deploymentProcessor) getResource(ctx context.Context, id string, out interface{}) (etag string, err error) {
	etag = ""
	var res *store.Object
	if res, err = dp.db.Get(ctx, id); err == nil {
		if err = res.As(out); err == nil {
			etag = res.ETag
			return
		}
	}
	return
}
