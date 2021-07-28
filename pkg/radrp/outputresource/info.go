// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	Resource           interface{}
	Deployed           bool
	LocalID            string
	Managed            bool
	ResourceKind       string
	OutputResourceType string
	OutputResourceInfo interface{}
}

// ARMInfo info required to deploy an ARM resource
type ARMInfo struct {
	ARMID           string `bson:"armId"`
	ARMResourceType string `bson:"armResourceType"`
	APIVersion      string `bson:"apiVersion"`
}

// K8sInfo info required to deploy a Kubernetes resource
type K8sInfo struct {
	Kind       string `bson:"kind"`
	APIVersion string `bson:"apiVersion"`
	Name       string `bson:"name"`
	Namespace  string `bson:"namespace"`
}

// AADPodIdentity pod identity for AKS cluster to enable access to keyvault
type AADPodIdentity struct {
	AKSClusterName string `bson:"aadPodIdentity"`
	Name           string `bson:"name"`
	Namespace      string `bson:"namespace"`
}
