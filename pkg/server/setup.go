package server

import (
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

func NewNamespace(namespace string) *ProviderNamespace {
	return &ProviderNamespace{
		ProviderName:  namespace,
		ResourceTypes: make(map[string]any),
	}
}

func SetupNamespace() {
	ns := NewNamespace("Applications.Core")
	ns.AddOperations()
	ns.AddResourceType("environments", Options{
		RequestConverter:  converter.EnvironmentDataModelFromVersioned,
		ResponseConverter: converter.EnvironmentDataModelToVersioned,

		ListHandler: {
			APIHandler: defaultoperation.NewListResource,
		},

		GetHandler: {
			APIHandler: defaultoperation.NewListResource,
		},

		PutHandler: {
			APIHandler: defaultoperation.NewPutHandler,
			Filters:    {},
			JobHandler: deploymentprocessor,
		},

		PatchHandler: {
			APIHandler: defaultoperation.NewPatchHandler,
			Filters:    {},
			JobHandler: deploymentprocessor,
		},

		DeleteHandler: {
			APIHandler: defaultoperation.NewPatchHandler,
			Filters:    {},
			JobHandler: deploymentprocessor,
		},

		CustomActionHandlers: map[string]Handlers{
			"listSecrets": NewListSecrets,
		},
	})

	ns.AddResourceType("containers", Options{
		RequestConverter:  converter.EnvironmentDataModelFromVersioned,
		ResponseConverter: converter.EnvironmentDataModelToVersioned,

		PutHandler: {
			Filters:    {},
			JobHandler: deploymentprocessor,
		},
		PatchHandler: {
			Filters:    {},
			JobHandler: deploymentprocessor,
		},
		DeleteHandler: {
			Filters:    {},
			JobHandler: deploymentprocessor,
		},
	})

	ns.Build(apiService, asyncWorker)
}
