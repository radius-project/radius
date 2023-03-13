// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"testing"

	model "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	kind          = "kubernetes"
	namespace     = "default"
	appNamespace  = "app-default"
	envResourceId = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0"
	appResourceId = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/app0"
	azureScope    = "/subscriptions/test-sub/resourceGroups/testRG"
	awsScope      = "/planes/aws/aws/accounts/000/regions/cool-region"
)

func Test_GetConfigurationAzure(t *testing.T) {
	envConfig := &Configuration{
		Runtime: RuntimeConfiguration{
			Kubernetes: &KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: createAzureProvider(),
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
					Scope: to.Ptr(azureScope),
				},
			},
		},
	}
	result, err := getConfiguration(&envResource, nil)
	require.NoError(t, err)
	require.Equal(t, envConfig, result)
}

func Test_GetConfigurationAWS(t *testing.T) {
	envConfig := &Configuration{
		Runtime: RuntimeConfiguration{
			Kubernetes: &KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: createAWSProvider(),
	}
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
			Providers: &model.Providers{
				Aws: &model.ProvidersAws{
					Scope: to.Ptr(awsScope),
				},
			},
		},
	}
	result, err := getConfiguration(&envResource, nil)
	require.NoError(t, err)
	require.Equal(t, envConfig, result)

	appConfig := &Configuration{
		Runtime: RuntimeConfiguration{
			Kubernetes: &KubernetesRuntime{
				Namespace: "app-default",
			},
		},
		Providers: createAWSProvider(),
	}
	appResource := model.ApplicationResource{
		Properties: &model.ApplicationProperties{
			Status: &model.ResourceStatus{
				Compute: &model.KubernetesCompute{
					Kind:       to.Ptr(kind),
					Namespace:  to.Ptr(appNamespace),
					ResourceID: to.Ptr(appResourceId),
				},
			},
		},
	}
	result, err = getConfiguration(&envResource, &appResource)
	require.NoError(t, err)
	require.Equal(t, appConfig, result)
}

func Test_InvalidApplicationError(t *testing.T) {
	envResource := model.EnvironmentResource{
		Properties: &model.EnvironmentProperties{
			Compute: &model.KubernetesCompute{
				Kind:       to.Ptr(kind),
				Namespace:  to.Ptr(namespace),
				ResourceID: to.Ptr(envResourceId),
			},
		},
	}
	// Invalid app model (should have KubernetesCompute field)
	appResource := model.ApplicationResource{
		Properties: &model.ApplicationProperties{
			Status: &model.ResourceStatus{
				Compute: &model.EnvironmentCompute{},
			},
		},
	}
	_, err := getConfiguration(&envResource, &appResource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid model conversion")
}

func createAzureProvider() map[string]map[string]any {
	return map[string]map[string]any{
		resourcemodel.ProviderAzure: {
			"scope": azureScope,
		}}
}

func createAWSProvider() map[string]map[string]any {
	return map[string]map[string]any{
		resourcemodel.ProviderAWS: {
			"scope": awsScope,
		}}
}
