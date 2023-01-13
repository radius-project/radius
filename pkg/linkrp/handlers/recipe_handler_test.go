// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_ContextParameter(t *testing.T) {
	linkID, err := resources.ParseResource("/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0")
	require.NoError(t, err)
	contextMeta := datamodel.RecipeContextMetaData{
		LinkID:               linkID,
		EnvironmentID:        "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		ApplicationID:        "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		EnvironmentNamespace: "radius-test-env",
		ApplicationNamespace: "radius-test-app",
	}
	expectedLinkContext := datamodel.RecipeContext{
		Link: datamodel.Link{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
			Name: "mongo0",
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
				ApplicationNamespace: "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	}
	expectedLinkContext.Timestamp = strconv.FormatInt(time.Now().UnixMilli(), 10)
	linkContext, err := createContextParameter(&contextMeta)
	require.NoError(t, err)
	require.Equal(t, expectedLinkContext, *linkContext)
}

func Test_ContextParameterError(t *testing.T) {
	linkID, err := resources.ParseResource("/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0")
	require.NoError(t, err)
	contextMeta := datamodel.RecipeContextMetaData{
		LinkID:               linkID,
		EnvironmentID:        "error-env",
		ApplicationID:        "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		EnvironmentNamespace: "radius-test-env",
		ApplicationNamespace: "radius-test-app",
	}

	linkContext, err := createContextParameter(&contextMeta)
	require.Error(t, err)
	require.Equal(t, fmt.Errorf("failed to parse EnvironmentID : %q while building the context parameter", contextMeta.EnvironmentID), err)
	require.Nil(t, linkContext)
}
