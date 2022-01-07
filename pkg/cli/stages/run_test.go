// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package stages

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/cli/builders"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/radyaml"
	"github.com/Azure/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func SkipBicepBuild(ctx context.Context, deployFile string) (string, error) {
	// We don't want to run bicep in unit tests. It's fine because we're not going to
	// look at the output of this.
	return "", nil
}

func Test_EmptyRadYaml_DoesNotCrash(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name:   "test",
		Stages: []radyaml.Stage{},
	}

	tempDir := t.TempDir()
	options := Options{
		Environment:    &MockEnvironment{},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)
	require.Empty(t, results)
}

func Test_MissingStage_ReturnsError(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name:   "test",
		Stages: []radyaml.Stage{},
	}

	tempDir := t.TempDir()
	options := Options{
		Environment:    &MockEnvironment{},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "missing",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.Error(t, err)
	require.Equal(t, "stage \"missing\" not found in rad.yaml", err.Error())
	require.Empty(t, results)
}

func Test_CanSkipStageWithNothingToDo(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
			},
		},
	}

	tempDir := t.TempDir()
	options := Options{
		Environment:    &MockEnvironment{},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage:  &manifest.Stages[0],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
	}
	require.Equal(t, expected, results)
}

func Test_CanRunAllStages(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
			},
			{
				Name: "second",
			},
			{
				Name: "third",
			},
		},
	}

	tempDir := t.TempDir()
	options := Options{
		Environment:    &MockEnvironment{},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage:  &manifest.Stages[0],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
		{
			Stage:  &manifest.Stages[1],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
		{
			Stage:  &manifest.Stages[2],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
	}
	require.Equal(t, expected, results)
}

func Test_CanSpecifyLastStage(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
			},
			{
				Name: "second",
			},
			{
				Name: "third",
			},
		},
	}

	tempDir := t.TempDir()
	options := Options{
		Environment:    &MockEnvironment{},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "third",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage:  &manifest.Stages[0],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
		{
			Stage:  &manifest.Stages[1],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
		{
			Stage:  &manifest.Stages[2],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
	}
	require.Equal(t, expected, results)
}

func Test_CanSpecifyStage(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
			},
			{
				Name: "second",
			},
			{
				Name: "third",
			},
		},
	}

	tempDir := t.TempDir()
	options := Options{
		Environment:    &MockEnvironment{},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "second",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage:  &manifest.Stages[0],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
		{
			Stage:  &manifest.Stages[1],
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
	}
	require.Equal(t, expected, results)
}

func Test_CanPropagateParameters(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
				Bicep: &radyaml.BicepStage{
					Template: to.StringPtr("iac/first.bicep"),
				},
			},
			{
				Name: "second",
				Bicep: &radyaml.BicepStage{
					Template: to.StringPtr("iac/first.bicep"),
				},
			},
		},
	}

	tempDir := t.TempDir()
	err := os.MkdirAll(path.Join(tempDir, "iac"), 0755)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(tempDir, "iac", "first.bicep"), []byte(""), 0644)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(tempDir, "iac", "second.bicep"), []byte(""), 0644)
	require.NoError(t, err)

	options := Options{
		Environment: &MockEnvironment{
			DeploymentClient: &MockDeploymentClient{
				Results: []clients.DeploymentResult{
					{
						Outputs: map[string]clients.DeploymentOutput{
							"param2": {
								Type:  "string",
								Value: "value2",
							},
						},
					},
					{
						Outputs: map[string]clients.DeploymentOutput{
							"param3": {
								Type:  "string",
								Value: "value3",
							},
						},
					},
				},
			},
		},
		BaseDirectory: tempDir,
		Manifest:      manifest,
		FinalStage:    "second",
		Parameters: map[string]map[string]interface{}{
			"param1": {
				"value": "value1",
			},
		},
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage: &manifest.Stages[0],
			Input: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
			},
			Output: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
				"param2": {
					"value": "value2",
				},
			},
		},
		{
			Stage: &manifest.Stages[1],
			Input: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
				"param2": {
					"value": "value2",
				},
			},
			Output: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
				"param2": {
					"value": "value2",
				},
				"param3": {
					"value": "value3",
				},
			},
		},
	}
	require.Equal(t, expected, results)
}

func Test_CanOverrideStage(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
				Bicep: &radyaml.BicepStage{
					// NOTE: we don't create this file, so the test will fail
					// if the code tries to use it.
					Template: to.StringPtr("iac/first.bicep"),
				},
				Profiles: map[string]radyaml.Profile{
					"dev": {
						Bicep: &radyaml.BicepStage{
							Template: to.StringPtr("iac/first-dev.bicep"),
						},
					},
				},
			},
		},
	}

	tempDir := t.TempDir()
	err := os.MkdirAll(path.Join(tempDir, "iac"), 0755)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(tempDir, "iac", "first-dev.bicep"), []byte(""), 0644)
	require.NoError(t, err)

	options := Options{
		Environment: &MockEnvironment{
			DeploymentClient: &MockDeploymentClient{},
		},
		BaseDirectory:  tempDir,
		Manifest:       manifest,
		FinalStage:     "first",
		Profile:        "dev",
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage: &radyaml.Stage{
				Name:     "first",
				Profiles: manifest.Stages[0].Profiles,
				Bicep: &radyaml.BicepStage{
					Template: to.StringPtr("iac/first-dev.bicep"),
				},
			},
			Input:  map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{},
		},
	}
	require.Equal(t, expected, results)
}

var _ environments.DeploymentEnvironment = (*MockEnvironment)(nil)
var _ environments.DiagnosticsEnvironment = (*MockEnvironment)(nil)

type MockEnvironment struct {
	environments.GenericEnvironment
	DeploymentClient  clients.DeploymentClient
	DiagnosticsClient clients.DiagnosticsClient
}

func (e *MockEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	return e.DeploymentClient, nil
}

func (e *MockEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	return e.DiagnosticsClient, nil
}

var _ clients.DeploymentClient = (*MockDeploymentClient)(nil)

type MockDeploymentClient struct {
	count   int
	Results []clients.DeploymentResult
}

func (dc *MockDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	if options.ProgressChan != nil {
		close(options.ProgressChan)
	}

	result := clients.DeploymentResult{}
	if len(dc.Results) > dc.count {
		result = dc.Results[dc.count]
	}

	dc.count++
	return result, nil
}

var _ builders.Builder = (*MockBuilder)(nil)

type MockBuilder struct {
	count   int
	Results []builders.Output
}

func (mb *MockBuilder) Build(ctx context.Context, options builders.Options) (builders.Output, error) {
	result := builders.Output{}
	if len(mb.Results) > mb.count {
		result = mb.Results[mb.count]
	}

	mb.count++
	return result, nil
}

func Test_CanUseBuildResults(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	manifest := radyaml.Manifest{
		Name: "test",
		Stages: []radyaml.Stage{
			{
				Name: "first",
				Bicep: &radyaml.BicepStage{
					Template: to.StringPtr("iac/first.bicep"),
				},
			},
			{
				Name: "second",
				Build: map[string]*radyaml.BuildTarget{
					"todoapp": {
						Builder: "test-builder",
						Values:  map[string]interface{}{},
					},
				},
				Bicep: &radyaml.BicepStage{
					Template: to.StringPtr("iac/second.bicep"),
				},
			},
		},
	}

	tempDir := t.TempDir()
	err := os.MkdirAll(path.Join(tempDir, "iac"), 0755)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(tempDir, "iac", "first.bicep"), []byte(""), 0644)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(tempDir, "iac", "second.bicep"), []byte(""), 0644)
	require.NoError(t, err)

	options := Options{
		Environment: &MockEnvironment{
			DeploymentClient: &MockDeploymentClient{
				Results: []clients.DeploymentResult{
					{
						Outputs: map[string]clients.DeploymentOutput{
							"param1": {
								Type:  "string",
								Value: "value1",
							},
						},
					},
				},
			},
		},
		BaseDirectory: tempDir,
		Manifest:      manifest,
		Builders: map[string]builders.Builder{
			"test-builder": &MockBuilder{
				Results: []builders.Output{
					{
						Result: "build-result",
					},
				},
			},
		},
		BicepBuildFunc: SkipBicepBuild,
	}

	results, err := Run(ctx, options)
	require.NoError(t, err)

	expected := []StageResult{
		{
			Stage: &manifest.Stages[0],
			Input: map[string]map[string]interface{}{},
			Output: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
			},
		},
		{
			Stage: &manifest.Stages[1],
			Input: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
			},
			Output: map[string]map[string]interface{}{
				"param1": {
					"value": "value1",
				},
				"todoapp": {
					"value": "build-result",
				},
			},
		},
	}
	require.Equal(t, expected, results)
}
