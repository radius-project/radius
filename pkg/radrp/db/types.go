// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/revision"
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
	Properties   map[string]interface{} `bson:"properties,omitempty"`
	Components   map[string]Component   `bson:"components,omitempty"`
	Scopes       map[string]Scope       `bson:"scopes,omitempty"`
	Deployments  map[string]Deployment  `bson:"deployments,omitempty"`
}

// ApplicationPatch represents an Radius application without its nested resources.
type ApplicationPatch struct {
	ResourceBase `bson:",inline"`
	Properties   map[string]interface{} `bson:"properties,omitempty"`
}

// Component represents an Radius Component.
type Component struct {
	ResourceBase    `bson:",inline"`
	Kind            string              `bson:"kind"`
	Revision        revision.Revision   `bson:"revision,omitempty"`
	Properties      ComponentProperties `bson:"properties,omitempty"`
	OutputResources []OutputResource    `bson:"outputresources,omitempty"`
}

// ComponentProperties represents the properties of an Radius Component.
type ComponentProperties struct {
	Build    map[string]interface{}      `bson:"build,omitempty"`
	Config   map[string]interface{}      `bson:"config,omitempty"`
	Run      map[string]interface{}      `bson:"run,omitempty"`
	Bindings map[string]ComponentBinding `bson:"provides,omitempty"`
	Uses     []ComponentDependency       `bson:"dependsOn,omitempty"`
	Traits   []ComponentTrait            `bson:"traits,omitempty"`
}

// ComponentBinding represents a binding provided by an Radius Component.
type ComponentBinding struct {
	Kind                 string                 `bson:"kind"`
	AdditionalProperties map[string]interface{} `bson:",inline"`
}

// ComponentDependency represents a binding used by an Radius Component.
type ComponentDependency struct {
	Binding components.BindingExpression            `bson:"binding"`
	Env     map[string]components.BindingExpression `bson:"env,omitempty"`
	Secrets *ComponentDependencySecrets             `bson:"secrets,omitempty"`
}

// ComponentDependencySecrets represents actions to take on a secret store as part of a binding.
type ComponentDependencySecrets struct {
	Store components.BindingExpression            `bson:"store"`
	Keys  map[string]components.BindingExpression `bson:"keys,omitempty"`
}

// ComponentTrait represents a trait for an Radius component.
type ComponentTrait struct {
	Kind                 string                 `bson:"kind"`
	AdditionalProperties map[string]interface{} `bson:",inline"`
}

// OutputResource represents an output resource comprising a Radius component.
type OutputResource struct {
	LocalID            string      `bson:"id"`
	ResourceKind       string      `bson:"resourcekind"`
	OutputResourceInfo interface{} `bson:"outputresourceinfo"`
	Managed            string      `bson:"managed"`
	OutputResourceType string      `bson:"outputresourcetype"`
	Resource           interface{} `bson:"resource"`
}

// ArmInfo contains the details of the ARM resource
// when the DeploymentResource is an ARM resource
type ArmInfo struct {
	ArmID           string `bson:"armid"`
	ArmResourceType string `bson:"armresourcetype"`
	APIVersion      string `bson:"apiversion"`
}

// K8sInfo contains the details of the Kubernetes resource
// when the DeploymentResource is a Kubernetes resource
type K8sInfo struct {
	Kind       string `bson:"kind"`
	APIVersion string `bson:"apiversion"`
	Name       string `bson:"name"`
	Namespace  string `bson:"namespace"`
}

// AADPodIdentity contains the details of the Pod Identity resource
// when the DeploymentResource is a an AAD Pod identity
type AADPodIdentity struct {
	AKSClusterName string `bson:"aadpodidentity"`
	Name           string `bson:"name"`
	Namespace      string `bson:"namespace"`
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
	Workloads []DeploymentWorkload `bson:"workloads,omitempty"`
}

// DeploymentWorkload represents the status of a deployed workload.
type DeploymentWorkload struct {
	ComponentName   string               `bson:"componentName"`
	Kind            string               `bson:"kind"`
	Resources       []DeploymentResource `bson:"resources,omitempty"`
	OutputResources []OutputResource     `bson:"outputresources,omitempty"`
}

// DeploymentService represents the status of a deployed service.
type DeploymentService struct {
	Name       string                 `bson:"name"`
	Kind       string                 `bson:"kind"`
	Provider   string                 `bson:"provider"`
	Properties map[string]interface{} `bson:"properties"`
}

// DeploymentResource Types
const (
	ArmType        = "Arm"
	KubernetesType = "Kubernetes"
	PodIdentity    = "PodIdentity"
)

// DeploymentResource represents a deployed resource by Radius.
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
		Components:  map[string]Component{},
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

// AssignRevisions stamps the latest version of component into the deployment unless otherwise specified - also
// grab the 'active' version of each component
func (d Deployment) AssignRevisions(app *Application) (map[string]revision.Revision, error) {
	revisions := map[string]revision.Revision{}

	for _, dc := range d.Properties.Components {
		name := dc.FriendlyName()
		component, ok := app.Components[name]
		if !ok {
			return nil, fmt.Errorf("component %s does not exist", name)
		}

		// Use the latest
		dc.Revision = component.Revision
		revisions[name] = component.Revision
	}

	return revisions, nil
}

// GetRevisions gets the deployed revision for each component that is part of the deployment. This should
// only be called on a deployment that's been deployed already.
func (d Deployment) GetRevisions() map[string]revision.Revision {
	revisions := map[string]revision.Revision{}

	for _, dc := range d.Properties.Components {
		name := dc.FriendlyName()
		revisions[name] = dc.Revision
	}

	return revisions
}
