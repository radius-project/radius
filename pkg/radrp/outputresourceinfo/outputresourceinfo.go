// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresourceinfo

// OutputResource Types
// This is not a fixed set
const (
	TypeARM         = "ARM"
	TypeKubernetes  = "Kubernetes"
	TypePodIdentity = "PodIdentity"
)

// ARMInfo contains the details of the ARM resource
// when the DeploymentResource is an ARM resource
type ARMInfo struct {
	ARMID           string `bson:"armId"`
	ARMResourceType string `bson:"armResourceType"`
	APIVersion      string `bson:"apiVersion"`
}

// K8sInfo contains the details of the Kubernetes resource
// when the DeploymentResource is a Kubernetes resource
type K8sInfo struct {
	Kind       string `bson:"kind"`
	APIVersion string `bson:"apiVersion"`
	Name       string `bson:"name"`
	Namespace  string `bson:"namespace"`
}

// AADPodIdentity contains the details of the Pod Identity resource
// when the DeploymentResource is a an AAD Pod identity
type AADPodIdentity struct {
	AKSClusterName string `bson:"aadPodIdentity"`
	Name           string `bson:"name"`
	Namespace      string `bson:"namespace"`
}
