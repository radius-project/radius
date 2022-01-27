// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/cli/armtemplate/providers"
	"github.com/stretchr/testify/require"
)

func Test_ResolveType(t *testing.T) {
	type input struct {
		Provider     string
		Version      string
		ResourceType string
		Expected     string
	}

	ps := map[string]providers.Provider{
		"test":                             &FuncProvider{},
		providers.AzureProviderImport:      &FuncProvider{},
		providers.DeploymentProviderImport: &FuncProvider{},
		providers.KubernetesProviderImport: &FuncProvider{},
		providers.RadiusProviderImport:     &FuncProvider{},
	}

	inputs := []input{
		{
			Expected:     "test",
			Provider:     "test", // Lookup by provider ID
			ResourceType: "Lookup.By/provider",
		},
		{
			Expected:     providers.AzureProviderImport,
			Provider:     "",
			ResourceType: "Microsoft.Storage/account",
		},
		{
			Expected:     providers.DeploymentProviderImport,
			Provider:     "",
			ResourceType: DeploymentResourceType,
		},
		{
			Expected:     providers.KubernetesProviderImport,
			Provider:     "",
			ResourceType: "kubernetes.core/Secret",
		},
		{
			Expected:     providers.KubernetesProviderImport,
			Provider:     "",
			ResourceType: "kubernetes.apps/Deployment",
		},
		{
			Expected:     providers.RadiusProviderImport,
			Provider:     "",
			ResourceType: "Microsoft.CustomProviders/resourceProviders",
		},
		{
			Expected:     providers.RadiusProviderImport,
			Provider:     "",
			ResourceType: "Microsoft.CustomProviders/resourceProviders/Applications",
		},
	}

	for _, test := range inputs {
		t.Run(test.ResourceType, func(t *testing.T) {
			actual, err := GetProvider(ps, test.Provider, test.Version, test.ResourceType)
			require.NoError(t, err)

			expected := ps[test.Expected]
			require.NotNil(t, expected)

			require.Same(t, expected, actual)
		})
	}
}

type FuncProvider struct {
	GetFunc    func(ctx context.Context, ref string, version string) (map[string]interface{}, error)
	DeployFunc func(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error)
	InvokeFunc func(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error)
}

func (p *FuncProvider) GetDeployedResource(ctx context.Context, ref string, version string) (map[string]interface{}, error) {
	return p.GetFunc(ctx, ref, version)
}

func (p *FuncProvider) DeployResource(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error) {
	return p.DeployFunc(ctx, id, version, body)
}

func (p *FuncProvider) InvokeCustomAction(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error) {
	return p.InvokeFunc(ctx, id, version, action, body)
}
