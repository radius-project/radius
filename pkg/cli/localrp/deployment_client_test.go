// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localrp

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/Azure/radius/pkg/cli/armtemplate/providers"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/stretchr/testify/require"
)

func Test_Integration(t *testing.T) {
	entries, err := os.ReadDir("testdata/")
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			baseDir := path.Join("testdata", entry.Name())
			client := &LocalRPDeploymentClient{
				SubscriptionID: "test-subscription",
				ResourceGroup:  "test-resource-group",

				// Mock all providers to use the filesystem
				Providers: map[string]providers.Provider{
					providers.AzureProviderImport:      &TestProvider{T: t, BaseDir: baseDir, Provider: providers.AzureProviderImport},
					providers.KubernetesProviderImport: &TestProvider{T: t, BaseDir: baseDir, Provider: providers.KubernetesProviderImport},
					providers.RadiusProviderImport:     &TestProvider{T: t, BaseDir: baseDir, Provider: providers.RadiusProviderImport},
				},
			}

			// Support nested deployments
			client.Providers[providers.DeploymentProviderImport] = &providers.DeploymentProvider{DeployFunc: client.DeployNested}

			require.FileExists(t, path.Join(baseDir, "template.json"))
			input, err := ioutil.ReadFile(path.Join(baseDir, "template.json"))
			require.NoError(t, err)

			parameters := map[string]map[string]interface{}{}
			parameterBytes, err := ioutil.ReadFile(path.Join(baseDir, "template.parameters.json"))
			if err != nil && !os.IsNotExist(err) {
				require.NoError(t, err)
			} else if err == nil {
				err = json.Unmarshal(parameterBytes, &parameters)
				require.NoError(t, err)
			}

			require.FileExists(t, path.Join(baseDir, "results.json"))
			expectedBytes, err := ioutil.ReadFile(path.Join(baseDir, "results.json"))
			require.NoError(t, err)

			expected := clients.DeploymentResult{}
			err = json.Unmarshal(expectedBytes, &expected)
			require.NoError(t, err)

			results, err := client.Deploy(context.Background(), clients.DeploymentOptions{
				Template:   string(input),
				Parameters: parameters,
			})
			require.NoErrorf(t, err, "failed to process deployment: %v", err)

			// We just compare the outputs (not the resource IDs) for ease of maintenance.
			require.Equal(t, expected.Outputs, results.Outputs)
		})
	}
}

var _ providers.Provider = (*TestProvider)(nil)

type TestProvider struct {
	T        *testing.T
	BaseDir  string
	Provider string
}

func (p *TestProvider) GetDeployedResource(ctx context.Context, id string, version string) (map[string]interface{}, error) {
	inputPath := path.Join(p.BaseDir, p.Provider, "output", strings.ReplaceAll(id, "/", "_")+".json")
	require.FileExists(p.T, inputPath)

	inputBytes, err := ioutil.ReadFile(inputPath)
	require.NoError(p.T, err)

	body := map[string]interface{}{}
	err = json.Unmarshal(inputBytes, &body)
	require.NoError(p.T, err)

	return body, nil
}

func (p *TestProvider) DeployResource(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error) {
	inputPath := path.Join(p.BaseDir, p.Provider, "input", strings.ReplaceAll(id, "/", "_")+".json")
	require.FileExists(p.T, inputPath)

	inputBytes, err := ioutil.ReadFile(inputPath)
	require.NoError(p.T, err)

	expectedBody := map[string]interface{}{}
	err = json.Unmarshal(inputBytes, &expectedBody)
	require.NoError(p.T, err)

	require.Equalf(p.T, expectedBody, body, "input comparison failed for PUT %s", id)

	outputPath := path.Join(p.BaseDir, p.Provider, "output", strings.ReplaceAll(id, "/", "_")+".json")
	require.FileExists(p.T, inputPath)

	outputBytes, err := ioutil.ReadFile(outputPath)
	require.NoError(p.T, err)

	outputBody := map[string]interface{}{}
	err = json.Unmarshal(outputBytes, &outputBody)
	require.NoError(p.T, err)

	return outputBody, nil
}

func (p *TestProvider) InvokeCustomAction(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error) {
	inputPath := path.Join(p.BaseDir, p.Provider, "input", strings.ReplaceAll(id, "/", "_")+"_"+action+".json")
	require.FileExists(p.T, inputPath)

	inputBytes, err := ioutil.ReadFile(inputPath)
	require.NoError(p.T, err)

	expectedBody := map[string]interface{}{}
	err = json.Unmarshal(inputBytes, &expectedBody)
	require.NoError(p.T, err)

	require.Equal(p.T, expectedBody, body, "input comparison failed for custom action %s on %s", action, id)

	outputPath := path.Join(p.BaseDir, p.Provider, "output", strings.ReplaceAll(id, "/", "_")+"_"+action+".json")
	require.FileExists(p.T, inputPath)

	outputBytes, err := ioutil.ReadFile(outputPath)
	require.NoError(p.T, err)

	outputBody := map[string]interface{}{}
	err = json.Unmarshal(outputBytes, &outputBody)
	require.NoError(p.T, err)

	return outputBody, nil
}
