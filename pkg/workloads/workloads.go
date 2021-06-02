// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"context"
	"errors"
	"strings"

	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/rest"
	"k8s.io/apimachinery/pkg/runtime"
)

// ErrUnknownType is the error reported when the workload type is unknown or unsupported.
var ErrUnknownType = errors.New("workload type is unsupported")

// InstantiatedWorkload workload provides all of the information needed to render a workload.
type InstantiatedWorkload struct {
	Application   string
	Name          string
	Workload      components.GenericComponent
	BindingValues map[components.BindingKey]components.BindingState
}

// WorkloadRenderer defines the interface for rendering a Kubernetes workload definition
// into a set of raw Kubernetes resources.
//
// The idea is that this represents *fan-out* in terms of the implementation. All of the APIs here
// could be replaced with REST calls.
type WorkloadRenderer interface {
	// AllocateBindings is called for the component to provide its supported bindings and their values.
	AllocateBindings(ctx context.Context, workload InstantiatedWorkload, resources []WorkloadResourceProperties) (map[string]components.BindingState, error)
	// Render is called for the component to provide its output resources.
	Render(ctx context.Context, workload InstantiatedWorkload) ([]WorkloadResource, []rest.RadResource, error)
}

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	Resource           interface{}
	Created            bool   // TODO: Temporary workaround till some resources are created in Render phase
	LocalID            string // Resources need to be tracked even before actually creating them. Local ID provides a way to track them.
	Managed            string
	ResourceKind       string
	OutputResourceType string
	OutputResourceInfo interface{}
}

// ArmInfo contains the details of an output ARM resource
type ArmInfo struct {
	ResourceID   string
	ResourceType string
	APIVersion   string
}

// CreateArmResource returns an object of type OutputResource initialized with the data from the ARM resource
func CreateArmResource(created bool, resourceKind, id string, resourceType string, managed bool, localIDPrefix string) OutputResource {
	armInfo := ArmInfo{
		ResourceID:   id,
		ResourceType: resourceType,
		APIVersion:   "???",
	}
	r := OutputResource{
		ResourceKind:       resourceKind,
		OutputResourceType: OutputResourceTypeArm,
		LocalID:            localidgenerator.MakeID(localIDPrefix),
		Managed:            "true",
		OutputResourceInfo: armInfo,
	}

	return r
}

// K8sInfo contains the details of an output Kubernetes resource
type K8sInfo struct {
	Kind       string
	APIVersion string
	Name       string
	Namespace  string
}

// CreateKubernetesResource returns an object of type OutputResource initialized with the data from the Kubernetes resource
func CreateKubernetesResource(created bool, resourceKind, kind, apiVersion, name, namespace, localIDPrefix, managed string, obj runtime.Object) OutputResource {
	k8sInfo := K8sInfo{
		Kind:       kind,
		APIVersion: apiVersion,
		Name:       name,
		Namespace:  namespace,
	}
	r := OutputResource{
		ResourceKind:       resourceKind,
		OutputResourceType: OutputResourceTypeKubernetes,
		LocalID:            localidgenerator.MakeID(localIDPrefix),
		Managed:            managed,
		OutputResourceInfo: k8sInfo,
		Resource:           obj,
	}

	return r
}

// AADPodIdentity contains the details of an output AAD Pod Identity resource
type AADPodIdentity struct {
	AKSClusterName string
	Name           string
	Namespace      string
}

const (
	PodIdentityName    = "podidentityname"
	PodIdentityCluster = "podidentitycluster"
)

// CreatePodIdentityResource returns an object of type OutputResource initialized with the data from the AADPodIdentity resource
func CreatePodIdentityResource(created bool, clusterName, name, namespace, localIDPrefix, managed string) OutputResource {
	podidInfo := AADPodIdentity{
		AKSClusterName: clusterName,
		Name:           name,
		Namespace:      namespace,
	}

	r := OutputResource{
		Created:            created,
		ResourceKind:       ResourceKindAzurePodIdentity,
		OutputResourceType: OutputResourceTypePodIdentity,
		LocalID:            localidgenerator.MakeID(localIDPrefix),
		Managed:            managed,
		OutputResourceInfo: podidInfo,
		Resource: map[string]string{
			PodIdentityName:    name,
			PodIdentityCluster: clusterName,
		},
	}

	return r
}

// WorkloadResourceProperties represents the properties output by deploying a resource.
type WorkloadResourceProperties struct {
	Type       string
	Properties map[string]string
}

// NewKubernetesResource creates a Kubernetes WorkloadResource
func NewKubernetesResource(localID string, obj runtime.Object) OutputResource {
	return OutputResource{ResourceKind: ResourceKindKubernetes, LocalID: localID, Resource: obj}
}

func (wr OutputResource) IsKubernetesResource() bool {
	return wr.ResourceKind == ResourceKindKubernetes
}

// GetOutputResourceType determines the deployment resource type
func (wr OutputResource) GetOutputResourceType() string {
	if wr.ResourceKind == ResourceKindAzurePodIdentity {
		return OutputResourceTypePodIdentity
	} else if strings.Contains(wr.ResourceKind, "azure") {
		return OutputResourceTypeArm
	} else if wr.ResourceKind == ResourceKindKubernetes {
		return OutputResourceTypeKubernetes
	} else {
		return ""
	}
}
