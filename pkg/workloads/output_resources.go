// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"github.com/Azure/radius/pkg/curp/localidgenerator"
	"k8s.io/apimachinery/pkg/runtime"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	Resource           interface{}
	Deployed           bool   // TODO: Temporary workaround till some resources are deployed in Render phase
	LocalID            string // Resources need to be tracked even before actually creating them. Local ID provides a way to track them.
	Managed            string
	ResourceKind       string
	OutputResourceType string
	OutputResourceInfo interface{}
}

// ARMInfo contains the details of an output ARM resource
type ARMInfo struct {
	ResourceID   string
	ResourceType string
	APIVersion   string
}

// CreateArmResource returns an object of type OutputResource initialized with the data from the ARM resource
func CreateArmResource(deployed bool, resourceKind, id string, resourceType string, apiversion string, managed bool, localIDPrefix string) OutputResource {
	armInfo := ARMInfo{
		ResourceID:   id,
		ResourceType: resourceType,
		APIVersion:   apiversion,
	}
	r := OutputResource{
		Deployed:           deployed,
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
func CreateKubernetesResource(deployed bool, resourceKind, kind, apiVersion, name, namespace, localIDPrefix, managed string, obj runtime.Object) OutputResource {
	k8sInfo := K8sInfo{
		Kind:       kind,
		APIVersion: apiVersion,
		Name:       name,
		Namespace:  namespace,
	}
	r := OutputResource{
		Deployed:           deployed,
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
func CreatePodIdentityResource(deployed bool, clusterName, name, namespace, localIDPrefix, managed string) OutputResource {
	podidInfo := AADPodIdentity{
		AKSClusterName: clusterName,
		Name:           name,
		Namespace:      namespace,
	}

	r := OutputResource{
		Deployed:           deployed,
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
