// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

import (
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/revision"
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
	ResourceBase `json:",inline"`
	Kind         string              `json:"kind"`
	Properties   ComponentProperties `json:"properties"`
}

// ComponentProperties represents the properties element of an Radius component.
type ComponentProperties struct {
	Revision  revision.Revision      `json:"revision"`
	Build     map[string]interface{} `json:"build,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
	Run       map[string]interface{} `json:"run,omitempty"`
	Provides  []ComponentProvides    `json:"provides,omitempty"`
	DependsOn []ComponentDependsOn   `json:"dependsOn,omitempty"`
	Traits    []ComponentTrait       `json:"traits,omitempty"`
}

// ComponentProvides represents a service provided by an Radius Component.
type ComponentProvides struct {
	Name string `json:"name"`
	Kind string `json:"kind"`

	// TODO this should support arbirary data
	Port          *int `json:"port,omitempty"`
	ContainerPort *int `json:"containerPort,omitempty"`
}

// ComponentDependsOn represents a service used by an Radius Component.
type ComponentDependsOn struct {
	Name string `json:"name"`
	Kind string `json:"kind"`

	// TODO this should support more settings
	SetEnv map[string]string `json:"setEnv,omitempty"`
}

// ComponentTrait represents a trait for an Radius component.
type ComponentTrait struct {
	Kind       string                 `json:"kind"`
	Properties map[string]interface{} `json:"properties,omitempty"`
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
	Components []DeploymentComponent `json:"components,omitempty"`
}

// DeploymentComponent respresents an entry for a component in a deployment.
type DeploymentComponent struct {
	ComponentName string                          `json:"componentName,omitempty"`
	ID            string                          `json:"id,omitempty"`
	Revision      revision.Revision               `json:"revision"`
	DataOutputs   []DeploymentComponentDataOutput `json:"dataOutputs,omitempty"`
	DataInputs    []DeploymentComponentDataInput  `json:"dataInputs,omitempty"`
	Traits        []DeploymentComponentTrait      `json:"traits,omitempty"`
	Scopes        []DeploymentComponentScope      `json:"scopes,omitempty"`
}

// DeploymentComponentTrait specifies a trait for a component as part of a deployment.
type DeploymentComponentTrait struct {
	Kind       string                 `json:"kind"`
	Properties map[string]interface{} `json:"properties"`
}

// DeploymentComponentScope specifies a scope for a component as part of a deployment.
type DeploymentComponentScope struct {
}

// DeploymentComponentDataOutput specifies a data output produced by a component as part of a deployment.
type DeploymentComponentDataOutput struct {
}

// DeploymentComponentDataInput specifies a data input needed by a component as part of a deployment.
type DeploymentComponentDataInput struct {
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
func (app *Application) SetID(id resources.ResourceID) {
	app.ResourceBase.ID = id.ID
	app.ResourceBase.Name = id.QualifiedName()
	app.ResourceBase.SubscriptionID = id.SubscriptionID
	app.ResourceBase.ResourceGroup = id.ResourceGroup
	app.ResourceBase.Type = id.Kind()
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
func (c *Component) SetID(id resources.ResourceID) {
	c.ResourceBase.ID = id.ID
	c.ResourceBase.Name = id.QualifiedName()
	c.ResourceBase.SubscriptionID = id.SubscriptionID
	c.ResourceBase.ResourceGroup = id.ResourceGroup
	c.ResourceBase.Type = id.Kind()
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
func (d *Deployment) SetID(id resources.ResourceID) {
	d.ResourceBase.ID = id.ID
	d.ResourceBase.Name = id.QualifiedName()
	d.ResourceBase.SubscriptionID = id.SubscriptionID
	d.ResourceBase.ResourceGroup = id.ResourceGroup
	d.ResourceBase.Type = id.Kind()
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
func (s *Scope) SetID(id resources.ResourceID) {
	s.ResourceBase.ID = id.ID
	s.ResourceBase.Name = id.QualifiedName()
	s.ResourceBase.SubscriptionID = id.SubscriptionID
	s.ResourceBase.ResourceGroup = id.ResourceGroup
	s.ResourceBase.Type = id.Kind()
}
