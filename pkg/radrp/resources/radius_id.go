// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/google/uuid"
)

// baseResourceType declares the base resource type for the Radius RP - all of the Radius resource types are children.
const baseResourceType = azresources.CustomProvidersResourceProviders

// applicationResourceType declares the resource type for an Application.
const applicationResourceType = "Applications"

// componentResourceType declares the resource type for a Component.
const componentResourceType = "Components"

// deploymentResourceType declares the resource type for a Deployment.
const deploymentResourceType = "Deployments"

// operationResourceType declares the resource type for an Operation.
const operationResourceType = "OperationResults"

// scopeResourceType declares the resource type for a Scope.
const scopeResourceType = "Scopes"

// We always deploy the Radius RP for appmodelv2 using the resource name 'radius'. This allows
// use to do versioning at the RP level since Custom RP does not support different resources per-api-version.
const appmodelv2RPName = "radius"

const V3ApplicationResourceType = "Application"
const V3OperationResourceType = operationResourceType

// ApplicationCollectionType can be used to validate resource IDs with ValidateResourceType.
var ApplicationCollectionType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType},
	},
}

// ApplicationResourceType can be used to validate resource IDs with ValidateResourceType.
var ApplicationResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
	},
}

// ComponentCollectionType can be used to validate resource IDs with ValidateResourceType.
var ComponentCollectionType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: componentResourceType},
	},
}

// ComponentResourceType can be used to validate resource IDs with ValidateResourceType.
var ComponentResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: componentResourceType, Name: "*"},
	},
}

// DeploymentCollectionType can be used to validate resource IDs with ValidateResourceType.
var DeploymentCollectionType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: deploymentResourceType},
	},
}

// DeploymentResourceType can be used to validate resource IDs with ValidateResourceType.
var DeploymentResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: deploymentResourceType, Name: "*"},
	},
}

// DeploymentResourceType can be used to validate resource IDs with ValidateResourceType.
var DeploymentOperationResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: deploymentResourceType, Name: "*"},
		{Type: operationResourceType, Name: "*"},
	},
}

// ScopeCollectionType can be used to validate resource IDs with ValidateResourceType.
var ScopeCollectionType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: scopeResourceType},
	},
}

// ScopeResourceType can be used to validate resource IDs with ValidateResourceType.
var ScopeResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{Type: baseResourceType, Name: appmodelv2RPName},
		{Type: applicationResourceType, Name: "*"},
		{Type: scopeResourceType, Name: "*"},
	},
}

// ResourceID represents the ID for a Radius resource.
type ResourceID struct {
	azresources.ResourceID
}

// ApplicationID represents the ResourceID for an application.
type ApplicationID struct {
	ResourceID
}

// ComponentID represents the ResourceID for a component.
type ComponentID struct {
	Resource ResourceID
	App      ApplicationID
}

// DeploymentID represents the ResourceID for a deployment.
type DeploymentID struct {
	Resource ResourceID
	App      ApplicationID
}

// ScopeID represents the ResourceID for a scope.
type ScopeID struct {
	Resource ResourceID
	App      ApplicationID
}

type DeploymentOperationID struct {
	Resource ResourceID
}

// Application gets an ApplicationID for the resource.
func (ri ResourceID) Application() (ApplicationID, error) {
	if len(ri.Types) < 2 ||
		!strings.EqualFold(ri.Types[0].Type, baseResourceType) ||
		!strings.EqualFold(ri.Types[1].Type, applicationResourceType) ||
		ri.Types[1].Name == "" {
		// Not a Radius resource type.
		return ApplicationID{}, errors.New("not an Application resource or child resource")
	}

	if len(ri.Types) == 2 {
		// Already an ApplicationID
		return ApplicationID{ri}, nil
	}

	// This is a Radius nested resource type, we need to make a new ID for the application.
	resourceID := ResourceID{
		ResourceID: azresources.ResourceID{
			ID:             azresources.MakeID(ri.SubscriptionID, ri.ResourceGroup, ri.Types[0], ri.Types[1]),
			SubscriptionID: ri.SubscriptionID,
			ResourceGroup:  ri.ResourceGroup,
			Types:          ri.Types[:2],
		},
	}
	return ApplicationID{
		ResourceID: resourceID,
	}, nil
}

// Component gets a ComponentID for the resource.
func (ri ResourceID) Component() (ComponentID, error) {
	err := ri.ValidateResourceType(ComponentResourceType)
	if err != nil {
		return ComponentID{}, fmt.Errorf("not a valid Component resource: %w", err)
	}

	app, err := ri.Application()
	if err != nil {
		return ComponentID{}, err
	}

	return ComponentID{ri, app}, nil
}

// Deployment gets a DeploymentID for the resource.
func (ri ResourceID) Deployment() (DeploymentID, error) {
	err := ri.ValidateResourceType(DeploymentResourceType)
	if err != nil {
		return DeploymentID{}, fmt.Errorf("not a valid Deployment resource: %w", err)
	}

	app, err := ri.Application()
	if err != nil {
		return DeploymentID{}, err
	}

	return DeploymentID{ri, app}, nil
}

// Scope gets a ScopeID for the resource.
func (ri ResourceID) Scope() (ScopeID, error) {
	err := ri.ValidateResourceType(ScopeResourceType)
	if err != nil {
		return ScopeID{}, fmt.Errorf("not a valid Scope resource: %w", err)
	}

	app, err := ri.Application()
	if err != nil {
		return ScopeID{}, err
	}

	return ScopeID{ri, app}, nil
}

// DeploymentOperation gets a DeploymentOperationID for the resource.
func (ri ResourceID) DeploymentOperation() (DeploymentOperationID, error) {
	err := ri.ValidateResourceType(DeploymentOperationResourceType)
	if err != nil {
		return DeploymentOperationID{}, fmt.Errorf("not a valid Deployment Operation resource: %w", err)
	}

	return DeploymentOperationID{Resource: ri}, nil
}

// Deployment gets a DeploymentID for the DeploymentOperationID resource.
func (d DeploymentOperationID) Deployment() (DeploymentID, error) {
	text := azresources.MakeID(
		d.Resource.SubscriptionID,
		d.Resource.ResourceGroup,
		d.Resource.Types[0],
		d.Resource.Types[1:3]...)
	id, err := azresources.Parse(text)
	if err != nil {
		return DeploymentID{}, fmt.Errorf("not a valid Deployment Operation resource: %w", err)
	}
	ri := ResourceID{id}
	return ri.Deployment()
}

// NewOperation creates a new (random) ID for an operation related to a deployment
func (di DeploymentID) NewOperation() DeploymentOperationID {
	name := uuid.New().String()
	id, err := azresources.Parse(di.Resource.ID + "/OperationResults/" + name)
	if err != nil {
		panic(err)
	}
	ri := ResourceID{id}
	return DeploymentOperationID{Resource: ri}
}
