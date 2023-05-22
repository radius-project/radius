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

package daprstatestores

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

func GetDaprStateStoreGeneric(resource *datamodel.DaprStateStore, applicationName string, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := resource.Properties

	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}

	outputResources, err := getDaprGeneric(daprGeneric, resource, applicationName, options.Namespace)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	return renderers.RendererOutput{
		Resources: outputResources,
	}, nil
}

func getDaprGeneric(daprGeneric dapr.DaprGeneric, dm v1.ResourceDataModel, applicationName string, namespace string) ([]rpv1.OutputResource, error) {
	err := daprGeneric.Validate()
	if err != nil {
		return nil, err
	}
	resource, ok := dm.(*datamodel.DaprStateStore)
	if !ok {
		return nil, v1.ErrInvalidModelConversion
	}
	daprGenericResource, err := dapr.ConstructDaprGeneric(daprGeneric, namespace, resource.Name, applicationName, resource.Name, linkrp.DaprStateStoresResourceType)
	if err != nil {
		return nil, err
	}

	output := rpv1.OutputResource{
		LocalID: rpv1.LocalIDDaprComponent,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprComponent,
			Provider: resourcemodel.ProviderKubernetes,
		},
		Resource: &daprGenericResource,
	}

	return []rpv1.OutputResource{output}, nil
}
