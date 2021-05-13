// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"strings"

	"github.com/Azure/radius/pkg/curp/armerrors"
	"github.com/Azure/radius/pkg/curp/revision"
)

// This package defines the data types that we store in the db - these are different from
// what we serialize over the wire.

// Resource Providers have some required fields:
// - id (fully-qualified resource id)
// - name
// - resourceGroup
// - location
// - tags
// - type
//
// The request submitted as a PUT won't include these as top-level properties, so we populate them before
// writing to the db. Additionally we store the subscriptionId as a top level property for ease of querying.
//
// We map the fully-qualified resource ID as the mongo `_id` field. This allows us to prevent duplicates.
//
// https://docs.microsoft.com/en-us/azure/azure-resource-manager/custom-providers/tutorial-custom-providers-function-authoring

// ResourceBase defines common properties for the Radius resource types.
type ResourceBase struct {
	ID             string            `bson:"_id"`
	Name           string            `bson:"name"`
	SubscriptionID string            `bson:"subscriptionId"`
	ResourceGroup  string            `bson:"resourceGroup"`
	Tags           map[string]string `bson:"tags"`
	Type           string            `bson:"type"`
	Location       string            `bson:"location"`
}

// Application represents an Radius Application with its nested resources.
type Application struct {
	ResourceBase `bson:",inline"`
	Properties   map[string]interface{}      `bson:"properties,omitempty"`
	Components   map[string]ComponentHistory `bson:"components,omitempty"`
	Scopes       map[string]Scope            `bson:"scopes,omitempty"`
	Deployments  map[string]Deployment       `bson:"deployments,omitempty"`
}

// ApplicationPatch represents an Radius application without its nested resources.
type ApplicationPatch struct {
	ResourceBase `bson:",inline"`
	Properties   map[string]interface{} `bson:"properties,omitempty"`
}

// ComponentHistory represents the whole history of a component.
type ComponentHistory struct {
	ResourceBase    `bson:",inline"`
	Revision        revision.Revision   `bson:"revision"`
	RevisionHistory []ComponentRevision `bson:"revisionHistory,omitempty"`
}

// ComponentRevision represents an individual revision of a component.
type ComponentRevision struct {
	Kind       string              `bson:"kind"`
	Revision   revision.Revision   `bson:"revision,omitempty"`
	Properties ComponentProperties `bson:"properties,omitempty"`
}

// Component represents an Radius Component.
type Component struct {
	ResourceBase `bson:",inline"`
	Kind         string              `bson:"kind"`
	Revision     revision.Revision   `bson:"revision,omitempty"`
	Properties   ComponentProperties `bson:"properties,omitempty"`
}

// ComponentProperties represents the properties of an Radius Component.
type ComponentProperties struct {
	Build     map[string]interface{} `bson:"build,omitempty"`
	Config    map[string]interface{} `bson:"config,omitempty"`
	Run       map[string]interface{} `bson:"run,omitempty"`
	Provides  []ComponentProvides    `bson:"provides,omitempty"`
	DependsOn []ComponentDependsOn   `bson:"dependsOn,omitempty"`
	Traits    []ComponentTrait       `bson:"traits,omitempty"`
}

// ComponentProvides represents a service provided by an Radius Component.
type ComponentProvides struct {
	Name string `bson:"name"`
	Kind string `bson:"kind"`

	// TODO this should support arbirary data
	Port          *int `bson:"port,omitemtpy"`
	ContainerPort *int `bson:"containerPort,omitempty"`
}

// ComponentDependsOn represents a service used by an Radius Component.
type ComponentDependsOn struct {
	Name string `bson:"name"`
	Kind string `bson:"kind"`

	// TODO this should support more settings
	SetEnv    map[string]string `bson:"setEnv,omitempty"`
	SetSecret map[string]string `bson:"setSecret,omitempty"`
}

// ComponentTrait represents a trait for an Radius component.
type ComponentTrait struct {
	Kind       string                 `bson:"kind"`
	Properties map[string]interface{} `bson:"properties,omitempty"`
}

// Scope represents an Radius Scope.
type Scope struct {
	ResourceBase `bson:",inline"`
	Properties   map[string]interface{} `bson:"properties,omitempty"`
}

// Deployment represents an Radius Deployment.
type Deployment struct {
	ResourceBase `bson:",inline"`
	Status       DeploymentStatus     `bson:"status"`
	Error        string               `bson:"error"`
	Properties   DeploymentProperties `bson:"properties"`
}

// DeploymentStatus represents the status of the deployment.
type DeploymentStatus struct {
	Services  map[string]DeploymentService `bson:"services"`
	Workloads []DeploymentWorkload         `bson:"workloads,omitempty"`
}

// DeploymentWorkload represents the status of a deployed workload.
type DeploymentWorkload struct {
	ComponentName string               `bson:"componentName"`
	Kind          string               `bson:"kind"`
	Resources     []DeploymentResource `bson:"resources,omitempty"`
}

// DeploymentService represents the status of a deployed service.
type DeploymentService struct {
	Name       string                 `bson:"name"`
	Kind       string                 `bson:"kind"`
	Provider   string                 `bson:"provider"`
	Properties map[string]interface{} `bson:"properties"`
}

// DeploymentResource represents a deployed kubernetes resource.
type DeploymentResource struct {
	LocalID    string            `bson:"id"`
	Type       string            `bson:"type"`
	Properties map[string]string `bson:"properties"`
}

// DeploymentProperties respresents the properties of a deployment.
type DeploymentProperties struct {
	ProvisioningState string                 `bson:"provisioningState"`
	Components        []*DeploymentComponent `bson:"components,omitempty" validate:"dive"`
}

// DeploymentComponent respresents an entry for a component in a deployment.
type DeploymentComponent struct {
	ComponentName string            `bson:"componentName,omitempty" validate:"required"`
	ID            string            `bson:"id,omitempty"`
	Revision      revision.Revision `bson:"revision"`
}

// See: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#asynchronous-operations
type Operation struct {
	ID     string `bson:"id"`
	Name   string `bson:"name"`
	Status string `bson:"status"`

	// These should be in ISO8601 format
	StartTime string `bson:"startTime"`
	EndTime   string `bson:"endTime"`

	PercentComplete float64                 `bson:"percentComplete"`
	Properties      map[string]interface{}  `bson:"properties,omitempty"`
	Error           *armerrors.ErrorDetails `bson:"error"`
}

// Marshal implements revision.Marshal for Component.
func (c *Component) Marshal() interface{} {
	return map[string]interface{}{
		"kind":       c.Kind,
		"properties": c.Properties,
	}
}

// Marshal implements revision.Marshal for Deployment.
func (d *Deployment) Marshal() interface{} {
	return map[string]interface{}{
		"properties": d.Properties,
	}
}

// NewApplication returns a new Application.
func NewApplication() *Application {
	return &Application{
		Properties:  map[string]interface{}{},
		Components:  map[string]ComponentHistory{},
		Scopes:      map[string]Scope{},
		Deployments: map[string]Deployment{},
	}
}

// FriendlyName gets the short name of the application.
func (app Application) FriendlyName() string {
	// use the last segment of the name
	if strings.Contains(app.Name, "/") {
		split := strings.Split(app.Name, "/")
		return split[len(split)-1]
	}

	return app.Name
}

// FriendlyName gets the short name of the application.
func (app ApplicationPatch) FriendlyName() string {
	// use the last segment of the name
	if strings.Contains(app.Name, "/") {
		split := strings.Split(app.Name, "/")
		return split[len(split)-1]
	}

	return app.Name
}

// LookupComponentRevision looks up the component revision by name and revision.
func (app Application) LookupComponentRevision(name string, revision revision.Revision) (*ComponentRevision, bool) {
	ch, ok := app.Components[name]
	if !ok {
		return nil, false
	}

	for _, cr := range ch.RevisionHistory {
		if cr.Revision == revision {
			return &cr, true
		}
	}

	return nil, false
}

// NewComponentProperties returns a new instance of ComponentProperties.
func NewComponentProperties() *ComponentProperties {
	return &ComponentProperties{
		Build:  map[string]interface{}{},
		Config: map[string]interface{}{},
		Run:    map[string]interface{}{},
	}
}

// NewDeployment returns a new Deployment.
func NewDeployment() *Deployment {
	return &Deployment{}
}

// Components returns the component instantiations of the deployment.
func (d Deployment) Components() []*DeploymentComponent {
	return d.Properties.Components
}

// LookupComponent returns the component instantiation looked up by friendly name.
func (d Deployment) LookupComponent(name string) (*DeploymentComponent, bool) {
	for _, c := range d.Properties.Components {
		if c.FriendlyName() == name {
			return c, true
		}
	}

	return nil, false
}

// FriendlyName gets the short name of the component reference.
func (dc DeploymentComponent) FriendlyName() string {
	name := ""
	if dc.ComponentName != "" {
		name = dc.ComponentName
	} else if dc.ID != "" {
		name = dc.ID
	}

	// use the last segment of the name
	if strings.Contains(name, "/") {
		split := strings.Split(name, "/")
		return split[len(split)-1]
	}

	return name
}
