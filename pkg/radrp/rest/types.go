// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import (
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/revision"
)

// This package defines the data types that we serialize over the wire - these are different from
// what we store in the db

// Resource represents an Azure resource.
type Resource interface {
	// Produces a resource ID from properties
	GetID() (resources.ResourceID, error)

	// Apply applies properties from a resource ID.
	SetID(id resources.ResourceID)
}

// ResourceList defines a list of resources.
type ResourceList struct {
	Value []interface{} `json:"value"`
}

// ResourceBase defines common properties for the Radius resource types.
type ResourceBase struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	SubscriptionID string            `json:"-"`
	ResourceGroup  string            `json:"resourceGroup"`
	Tags           map[string]string `json:"tags,omitempty"`
	Type           string            `json:"type"`
	Location       string            `json:"location,omitempty"`
}

// Application represents an Radius Application.
type Application struct {
	ResourceBase `json:",inline"`
	Properties   map[string]interface{} `json:"properties"`
}

// Component represents an Radius Component.
type Component struct {
	ResourceBase    `json:",inline"`
	Kind            string                     `json:"kind"`
	Properties      ComponentProperties        `json:"properties"`
	OutputResources []workloads.OutputResource `json:"outputresources"`
}

// ComponentProperties represents the properties element of an Radius component.
type ComponentProperties struct {
	Revision revision.Revision           `json:"revision"`
	Build    map[string]interface{}      `json:"build,omitempty"`
	Config   map[string]interface{}      `json:"config,omitempty"`
	Run      map[string]interface{}      `json:"run,omitempty"`
	Bindings map[string]ComponentBinding `json:"bindings,omitempty"`
	Uses     []ComponentDependency       `json:"uses,omitempty"`
	Traits   []ComponentTrait            `json:"traits,omitempty"`
}

// ComponentBinding represents a binding provided by an Radius Component.
type ComponentBinding struct {
	Kind                 string
	AdditionalProperties map[string]interface{}

	// ComponentBinding has custom marshaling code
}

// ComponentDependency represents a binding used by an Radius Component.
type ComponentDependency struct {
	Binding components.BindingExpression            `json:"binding"`
	Env     map[string]components.BindingExpression `json:"env,omitempty"`
	Secrets *ComponentDependencySecrets             `json:"secrets,omitempty"`
}

// ComponentDependencySecrets represents actions to take on a secret store as part of a binding.
type ComponentDependencySecrets struct {
	Store components.BindingExpression            `json:"store"`
	Keys  map[string]components.BindingExpression `json:"keys,omitempty"`
}

// ComponentTrait represents a trait for an Radius component.
type ComponentTrait struct {
	Kind                 string
	AdditionalProperties map[string]interface{}
}

// Scope represents an Radius Scope.
type Scope struct {
	ResourceBase `json:",inline"`
	Properties   map[string]interface{} `json:"properties"`
}

// Deployment represents an Radius Deployment.
type Deployment struct {
	ResourceBase `json:",inline"`
	Properties   DeploymentProperties `json:"properties"`
}

// DeploymentProperties respresents the properties of a deployment.
type DeploymentProperties struct {
	ProvisioningState OperationStatus       `json:"provisioningState,omitempty"`
	Components        []DeploymentComponent `json:"components,omitempty"`
}

// DeploymentComponent respresents an entry for a component in a deployment.
type DeploymentComponent struct {
	ComponentName string            `json:"componentName,omitempty"`
	ID            string            `json:"id,omitempty"`
	Revision      revision.Revision `json:"revision"`
}

// See: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#asynchronous-operations
type Operation struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`

	// These should be in ISO8601 format
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`

	PercentComplete float64                 `json:"percentComplete"`
	Properties      map[string]interface{}  `json:"properties,omitempty"`
	Error           *armerrors.ErrorDetails `json:"error"`
}

// See: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#asynchronous-operations
type OperationStatus string

const (
	// Terminal states
	SuccededStatus OperationStatus = "Succeeded"
	FailedStatus   OperationStatus = "Failed"
	CanceledStatus OperationStatus = "Canceled"

	// RP-defined statuses are used for non-terminal states
	DeployingStatus OperationStatus = "Deploying"
	DeletingStatus  OperationStatus = "Deleting"
)

func IsTeminalStatus(status OperationStatus) bool {
	return status == SuccededStatus || status == FailedStatus || status == CanceledStatus
}

// GetID produces a ResourceID from a resource.
func (app *Application) GetID() (resources.ResourceID, error) {
	return resources.Parse(app.ID)
}

// GetApplicationID produces a ApplicationID from a resource.
func (app *Application) GetApplicationID() (resources.ApplicationID, error) {
	ri, err := app.GetID()
	if err != nil {
		return resources.ApplicationID{}, err
	}

	return ri.Application()
}

// SetID applies the properties from a resource ID to the application.
func (app *Application) SetID(resource resources.ResourceID) {
	app.ResourceBase.ID = resource.ID
	app.ResourceBase.Name = resource.Name()
	app.ResourceBase.SubscriptionID = resource.SubscriptionID
	app.ResourceBase.ResourceGroup = resource.ResourceGroup
	app.ResourceBase.Type = resource.Kind()
}

// GetID produces a ResourceID from a resource.
func (c *Component) GetID() (resources.ResourceID, error) {
	return resources.Parse(c.ID)
}

// GetComponentID produces a ComponentID from a resource.
func (c *Component) GetComponentID() (resources.ComponentID, error) {
	ri, err := c.GetID()
	if err != nil {
		return resources.ComponentID{}, err
	}

	return ri.Component()
}

// SetID applies the properties from a resource ID to the component.
func (c *Component) SetID(resource resources.ResourceID) {
	c.ResourceBase.ID = resource.ID
	c.ResourceBase.Name = resource.Name()
	c.ResourceBase.SubscriptionID = resource.SubscriptionID
	c.ResourceBase.ResourceGroup = resource.ResourceGroup
	c.ResourceBase.Type = resource.Kind()
}

// GetID produces a ResourceID from a resource.
func (d *Deployment) GetID() (resources.ResourceID, error) {
	return resources.Parse(d.ID)
}

// GetDeploymentID produces a DeploymentID from a resource.
func (d *Deployment) GetDeploymentID() (resources.DeploymentID, error) {
	ri, err := d.GetID()
	if err != nil {
		return resources.DeploymentID{}, err
	}

	return ri.Deployment()
}

// SetID applies the properties from a resource ID to the deployment.
func (d *Deployment) SetID(resource resources.ResourceID) {
	d.ResourceBase.ID = resource.ID
	d.ResourceBase.Name = resource.Name()
	d.ResourceBase.SubscriptionID = resource.SubscriptionID
	d.ResourceBase.ResourceGroup = resource.ResourceGroup
	d.ResourceBase.Type = resource.Kind()
}

// GetID produces a ResourceID from a resource.
func (s *Scope) GetID() (resources.ResourceID, error) {
	return resources.Parse(s.ID)
}

// GetScopeID produces a ScopeID from a resource.
func (s *Scope) GetScopeID() (resources.ScopeID, error) {
	ri, err := s.GetID()
	if err != nil {
		return resources.ScopeID{}, err
	}

	return ri.Scope()
}

// SetID applies the properties from a resource ID to the scope.
func (s *Scope) SetID(resource resources.ResourceID) {
	s.ResourceBase.ID = resource.ID
	s.ResourceBase.Name = resource.Name()
	s.ResourceBase.SubscriptionID = resource.SubscriptionID
	s.ResourceBase.ResourceGroup = resource.ResourceGroup
	s.ResourceBase.Type = resource.Kind()
}
