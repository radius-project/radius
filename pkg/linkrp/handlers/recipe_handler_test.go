// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/stretchr/testify/require"
)

func Test_ParameterConflict(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	operatorParams := map[string]any{
		"port":     2040,
		"name":     "test-parameters-conflict",
		"location": "us-east1",
	}
	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"location": map[string]any{
			"value": "us-east1",
		},
	}

	actualParams := handleParameterConflict(devParams, operatorParams)
	require.Equal(t, expectedParams, actualParams)
}

func Test_ContextParameter(t *testing.T) {
	linkID := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0"
	expectedLinkContext := datamodel.RecipeContext{
		Resource: datamodel.Resource{
			ResourceInfo: datamodel.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.link/mongodatabases",
		},
		Application: datamodel.ResourceInfo{
			Name: "testApplication",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		},
		Environment: datamodel.ResourceInfo{
			Name: "env0",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		Runtime: datamodel.Runtime{
			Kubernetes: datamodel.Kubernetes{
				Namespace: "radius-test-app",
			},
		},
	}

	linkContext, err := CreateRecipeContextParameter(linkID, "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0", "radius-test-env", "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication", "radius-test-app")
	require.NoError(t, err)
	require.Equal(t, expectedLinkContext, *linkContext)
}

func Test_ContextParameterError(t *testing.T) {
	envID := "error-env"
	linkContext, err := CreateRecipeContextParameter("/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0", envID, "radius-test-env", "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication", "radius-test-app")
	require.Error(t, err)
	errMsg := fmt.Sprintf("%s is not a valid resource id", "'error-env'")
	require.Equal(t, fmt.Errorf("failed to parse environmentID : %q while building the context parameter %q", envID, errMsg), err)
	require.Nil(t, linkContext)
}
