// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metadata

import (
	"github.com/Azure/radius/pkg/workloads/cosmosdocumentdbv1alpha1"
	"github.com/Azure/radius/pkg/workloads/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/servicebusqueuev1alpha1"
)

// Registry defines the relationships between traits and workloads with built-in services.
type Registry struct {
	TraitServices        map[string]IntrinsicService
	WorkloadKindServices map[string]IntrinsicService
}

// IntrinsicService respresents a service defined intrisicly
type IntrinsicService struct {
	Kind string
}

// NewRegistry creates a metadata Registry
func NewRegistry() Registry {
	return Registry{
		TraitServices: map[string]IntrinsicService{
			"dapr.io/App@v1alpha1": {
				Kind: "dapr.io/Invoke",
			},
		},
		WorkloadKindServices: map[string]IntrinsicService{
			daprstatestorev1alpha1.Kind: {
				Kind: "dapr.io/StateStore",
			},
			daprpubsubv1alpha1.Kind: {
				Kind: "dapr.io/PubSubTopic",
			},
			cosmosdocumentdbv1alpha1.Kind: {
				Kind: "mongodb.com/Mongo",
			},
			servicebusqueuev1alpha1.Kind: {
				Kind: "azure.com/ServiceBusQueue",
			},
			"azure.com/KeyVault@v1alpha1": {
				Kind: "azure.com/KeyVault",
			},
		},
	}
}
