// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

// OutputResource Types
// This is not a fixed set
const (
	ArmType        = "Arm"
	KubernetesType = "Kubernetes"
	PodIdentity    = "PodIdentity"
)

// ARMInfo contains the details of the ARM resource
// when the DeploymentResource is an ARM resource
type ARMInfo struct {
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
