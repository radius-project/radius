/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package daprstatestores

import (
	"context"
	"errors"
	"fmt"
	"sort"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

type StateStoreFunc = func(resource *datamodel.DaprStateStore, applicationName string, options renderers.RenderOptions) (renderers.RendererOutput, error)

var SupportedStateStoreModes = map[string]StateStoreFunc{
	string(datamodel.LinkModeResource): GetDaprStateStoreAzureStorage,
	string(datamodel.LinkModeValues):   GetDaprStateStoreGeneric,
	string(datamodel.LinkModeRecipe):   GetDaprStateStoreRecipe,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	StateStores map[string]StateStoreFunc
}

func (r *Renderer) Render(ctx context.Context, dm v1.ResourceDataModel, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.DaprStateStore)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	properties := resource.Properties

	if r.StateStores == nil {
		return renderers.RendererOutput{}, errors.New("must support either kubernetes or ARM")
	}
	stateStoreFunc := r.StateStores[string(properties.Mode)]
	if stateStoreFunc == nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid state store mode, Supported mode values: %s", getAlphabeticallySortedKeys(r.StateStores)))
	}

	var applicationName string
	if properties.Application != "" {
		applicationID, err := renderers.ValidateApplicationID(properties.Application)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		applicationName = applicationID.Name()
	}

	rendererOutput, err := stateStoreFunc(resource, applicationName, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	values := map[string]renderers.ComputedValueReference{
		renderers.ComponentNameKey: {
			Value: kubernetes.NormalizeDaprResourceName(resource.Name),
		},
	}
	secrets := map[string]rpv1.SecretValueReference{}

	rendererOutput.ComputedValues = values
	rendererOutput.SecretValues = secrets
	return rendererOutput, nil
}

func getAlphabeticallySortedKeys(store map[string]StateStoreFunc) []string {
	keys := make([]string, len(store))

	i := 0
	for k := range store {
		keys[i] = k
		i++
	}

	sort.Strings(keys)
	return keys
}
