// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	Resource interface{}
	Kind     string
	LocalID  string
	Deployed bool
	Managed  bool
	Type     string
	Info     interface{}
}

// ARMInfo info required to identify an ARM resource
type ARMInfo struct {
	ID           string `bson:"id"`
	ResourceType string `bson:"resourceType"`
	APIVersion   string `bson:"apiVersion"`
}

// K8sInfo info required to identify a Kubernetes resource
type K8sInfo struct {
	Kind       string `bson:"kind"`
	APIVersion string `bson:"apiVersion"`
	Name       string `bson:"name"`
	Namespace  string `bson:"namespace"`
}

// AADPodIdentity pod identity for AKS cluster to enable access to keyvault
type AADPodIdentity struct {
	AKSClusterName string `bson:"aksClusterName"`
	Name           string `bson:"name"`
	Namespace      string `bson:"namespace"`
}
