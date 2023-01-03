// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rp

import (
	"context"
	"strings"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// EnvironmentComputeKind is the type of compute resource.
type EnvironmentComputeKind string

const (
	// UnknownComputeKind represents kubernetes compute resource type.
	UnknownComputeKind EnvironmentComputeKind = "unknown"
	// KubernetesComputeKind represents kubernetes compute resource type.
	KubernetesComputeKind EnvironmentComputeKind = "kubernetes"
)

// ComputedValueReference represents a non-secret value that can accessed once the output resources
// have been deployed.
type ComputedValueReference struct {
	// ComputedValueReference might hold a static value in `.Value` or might be a reference
	// that needs to be looked up.
	//
	// If `.Value` is set then treat this as a static value.
	//
	// If `.Value == nil` then use the `.PropertyReference` or to look up a property in the property
	// bag returned from deploying the resource via `handler.Put`.
	//
	// If `.Value == nil` && `.PropertyReference` is unset, then use JSONPointer to evaluate a JSON path
	// into the 'resource'.

	// LocalID specifies the output resource to be used for lookup. Does not apply with `.Value`
	LocalID string

	// Value specifies a static value to copy to computed values.
	Value any

	// PropertyReference specifies a property key to look up in the resource's *persisted properties*.
	PropertyReference string

	// JSONPointer specifies a JSON Pointer that cn be used to look up the value in the resource's body.
	JSONPointer string

	// Transformer transforms datamodel resource with the computed values.
	Transformer func(conv.DataModelInterface, map[string]any) error
}

// SecretValueReference represents a secret value that can accessed on the output resources
// have been deployed.
type SecretValueReference struct {
	// SecretValueReference always needs to be resolved against a deployed resource. These
	// are secrets so we don't want to store them.

	// LocalID is used to resolve the 'target' output resource for retrieving the secret value.
	LocalID string

	// Action refers to a named custom action used to fetch the secret value. Maybe be empty in the case of Kubernetes since there's
	// no concept of 'action'. Will always be set for an ARM resource.
	Action string

	// ValueSelector is a JSONPointer used to resolve the secret value.
	ValueSelector string

	// Transformer is a reference to a SecretValueTransformer that can be looked up by name.
	// By-convention this is the Resource Type of the resource (eg: Microsoft.DocumentDB/databaseAccounts).
	// If there are multiple kinds of transformers per Resource Type, then add a unique suffix.
	//
	// NOTE: the transformer is a string key because it has to round-trip from
	// the database. We don't store the secret value, so we have to be able to process it later.
	Transformer resourcemodel.ResourceType

	// Value is the secret value itself
	Value string
}

// SecretValueTransformer allows transforming a secret value before passing it on to a Resource
// that wants to access it.
//
// This is surprisingly common. For example, it's common for access control/connection strings to apply
// to an 'account' primitive such as a ServiceBus namespace or CosmosDB account. The actual connection
// string that application code consumes will include a database name or queue name, etc. Or the different
// libraries involved might support different connection string formats, and the user has to choose on.
type SecretValueTransformer interface {
	Transform(ctx context.Context, resourceComputedValues map[string]any, secretValue any) (any, error)
}

//go:generate mockgen -destination=./mock_secretvalueclient.go -package=rp -self_package github.com/project-radius/radius/pkg/rp github.com/project-radius/radius/pkg/rp SecretValueClient
type SecretValueClient interface {
	FetchSecret(ctx context.Context, identity resourcemodel.ResourceIdentity, action string, valueSelector string) (any, error)
}

// DeploymentOutput is the output details of a deployment.
type DeploymentOutput struct {
	DeployedOutputResources []outputresource.OutputResource
	ComputedValues          map[string]any
	SecretValues            map[string]SecretValueReference
}

// DeploymentDataModel is the interface that wraps existing data models
// and enables us to use in generic deployment backend controllers.
type DeploymentDataModel interface {
	conv.DataModelInterface

	ApplyDeploymentOutput(deploymentOutput DeploymentOutput)

	OutputResources() []outputresource.OutputResource
}

// BasicDaprResourceProperties is the basic resource properties for dapr resources.
type BasicDaprResourceProperties struct {
	// ComponentName represents the name of the component.
	ComponentName string `json:"componentName,omitempty"`
}

// BasicResourceProperties is the basic resource model for radius resources.
type BasicResourceProperties struct {
	// Environment represents the id of environment resource.
	Environment string `json:"environment,omitempty"`
	// Application represents the id of application resource.
	Application string `json:"application,omitempty"`

	// Status represents the resource status.
	Status ResourceStatus `json:"status,omitempty"`
}

// EqualLinkedResource returns true if the resource belongs to the same environment and application.
func (b *BasicResourceProperties) EqualLinkedResource(prop *BasicResourceProperties) bool {
	return strings.EqualFold(b.Application, prop.Application) && strings.EqualFold(b.Environment, prop.Environment)
}

type ResourceStatus struct {
	Compute         *EnvironmentCompute             `json:"compute,omitempty"`
	OutputResources []outputresource.OutputResource `json:"outputResources,omitempty"`
}

func (in *ResourceStatus) DeepCopy(out *ResourceStatus) {
	in.Compute = out.Compute
	in.OutputResources = out.OutputResources
}

// EnvironmentCompute represents the compute resource of Environment.
type EnvironmentCompute struct {
	Kind              EnvironmentComputeKind      `json:"kind"`
	KubernetesCompute KubernetesComputeProperties `json:"kubernetes,omitempty"`

	// Environment-level identity that can be used by any resource in the environment.
	// Resources can specify its own identities and they will override the environment-level identity.
	Identity *IdentitySettings `json:"identity,omitempty"`
}

// KubernetesComputeProperties represents the kubernetes compute of the environment.
type KubernetesComputeProperties struct {
	// ResourceID represents the resource ID for kuberentes compute resource.
	ResourceID string `json:"resourceId,omitempty"`

	// Namespace represents Kubernetes namespace.
	Namespace string `json:"namespace"`
}

// OutputResource contains some internal fields like resources/dependencies that shouldn't be inlcuded in the user response
func BuildExternalOutputResources(outputResources []outputresource.OutputResource) []map[string]any {
	var externalOutputResources []map[string]any
	for _, or := range outputResources {
		externalOutput := map[string]any{
			"LocalID":  or.LocalID,
			"Provider": or.ResourceType.Provider,
			"Identity": or.Identity.Data,
		}
		externalOutputResources = append(externalOutputResources, externalOutput)
	}

	return externalOutputResources
}

// RadiusResourceModel represents the interface of radius resource type.
// TODO: Replace DeploymentDataModel with RadiusResourceModel later when link rp leverages generic.
type RadiusResourceModel interface {
	conv.ResourceDataModel

	ApplyDeploymentOutput(deploymentOutput DeploymentOutput)
	OutputResources() []outputresource.OutputResource

	ResourceMetadata() *BasicResourceProperties
}
