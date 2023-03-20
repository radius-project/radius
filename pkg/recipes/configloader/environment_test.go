// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"testing"

	model "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	kind          = "kubernetes"
	namespace     = "default"
	envResourceId = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0"
	scope         = "/subscriptions/testSubs/resourceGroups/testRG"
)

func Test_GetConfiguration(t *testing.T) {
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: scope,
			},
		},
	}
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
			Providers: &model.Providers{
				Azure: &model.ProvidersAzure{
					Scope: to.Ptr(scope),
				},
			},
		},
	}

	result, err := getConfiguration(&envResource, nil)
	require.NoError(t, err)
	require.Equal(t, envConfig, result)
}
