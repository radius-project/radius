/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"github.com/go-chi/chi/v5"
	apictrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	ctr_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/containers"
	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
)

func NewNamespace(namespace string) *ProviderNamespace {
	return &ProviderNamespace{
		ResourceNode: ResourceNode{
			Name:     namespace,
			Children: make(map[string]*ResourceNode),
		},
		Router: chi.NewRouter(),
	}
}

func SetupNamespace() {
	ns := NewNamespace("Applications.Core")

	ns.AddChild("environments", &ResourceTypeHandlers{
		RequestConverter:  converter.EnvironmentDataModelFromVersioned_,
		ResponseConverter: converter.EnvironmentDataModelToVersioned_,

		Put: &ActionHandle[datamodel.Environment]{
			APIController: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		Patch: &ActionHandle[datamodel.Environment]{
			APIController: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		Custom: map[string]OperationHandler{
			"getmetadata": &ActionHandle[datamodel.Environment]{
				APIController: env_ctrl.NewGetRecipeMetadata,
			},
		},
	})

	ns.AddChild("containers", &ResourceTypeHandlers{
		RequestConverter:  converter.ContainerDataModelFromVersioned_,
		ResponseConverter: converter.ContainerDataModelToVersioned_,

		Put: &ActionHandle[datamodel.ContainerResource]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.ContainerResource]{
				rp_frontend.PrepareRadiusResource[*datamodel.ContainerResource],
				ctr_ctrl.ValidateAndMutateRequest,
			},
		},
		Patch: &ActionHandle[datamodel.ContainerResource]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.ContainerResource]{
				rp_frontend.PrepareRadiusResource[*datamodel.ContainerResource],
				ctr_ctrl.ValidateAndMutateRequest,
			},
		},
	})

	ns.Build(nil, nil)
}
