// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metadata

// Registry defines the relationships between traits and workloads with built-in services.
type Registry struct {
	TraitServices        map[string]IntrinsicService
	WorkloadKindServices map[string]IntrinsicService
}

// IntrinsicService respresents a service defined intrisicly
type IntrinsicService struct {
	Name string // TODO: remove Name from here.
	Kind string
}

// NewRegistry creates a metadata Registry
func NewRegistry() Registry {
	return Registry{
		TraitServices: map[string]IntrinsicService{
			"dapr.io/App@v1alpha1": {
				Name: "",
				Kind: "dapr.io/Invoke",
			},
		},
		WorkloadKindServices: map[string]IntrinsicService{
			"dapr.io/StateStore@v1alpha1": {
				Name: "",
				Kind: "dapr.io/StateStore",
			},
			"dapr.io/PubSub@v1alpha1": {
				Name: "",
				Kind: "dapr.io/PubSub",
			},
			"azure.com/CosmosDocumentDb@v1alpha1": {
				Name: "",
				Kind: "mongodb.com/Mongo",
			},
			"azure.com/ServiceBusQueue@v1alpha1": {
				Name: "",
				Kind: "azure.com/ServiceBusQueue",
			},
		},
	}
}
