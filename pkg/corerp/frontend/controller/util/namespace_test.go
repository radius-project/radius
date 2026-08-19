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

package util

import (
	"context"
	"errors"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testPlaneScope = "/planes/radius/local"

	envInDefaultGroup = "/planes/radius/local/resourcegroups/default/providers/Radius.Core/environments/default"
	envInOtherGroup   = "/planes/radius/local/resourcegroups/other/providers/Radius.Core/environments/other"
	legacyEnvID       = "/planes/radius/local/resourcegroups/default/providers/Applications.Core/environments/legacy"
)

// legacyEnvironment builds a stored Applications.Core/environments record.
func legacyEnvironment(id string, namespace string, kind rpv1.EnvironmentComputeKind) database.Object {
	env := &datamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{ID: id},
		},
	}
	env.Properties.Compute.Kind = kind
	env.Properties.Compute.KubernetesCompute.Namespace = namespace

	return database.Object{Metadata: database.Metadata{ID: id}, Data: env}
}

// previewEnvironment builds a stored Radius.Core/environments record.
func previewEnvironment(id string, namespace string) database.Object {
	env := &datamodel.Environment_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{ID: id},
		},
	}
	env.Properties.Providers = &datamodel.Providers_v20250801preview{
		Kubernetes: &datamodel.ProvidersKubernetes_v20250801preview{Namespace: namespace},
	}

	return database.Object{Metadata: database.Metadata{ID: id}, Data: env}
}

// expectQueries stubs the two namespace queries, returning legacyItems for the
// Applications.Core query and previewItems for the Radius.Core query.
func expectQueries(client *database.MockClient, legacyItems, previewItems []database.Object) {
	client.EXPECT().
		Query(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
			if strings.EqualFold(query.ResourceType, datamodel.EnvironmentResourceType) {
				return &database.ObjectQueryResult{Items: legacyItems}, nil
			}

			return &database.ObjectQueryResult{Items: previewItems}, nil
		}).AnyTimes()
}

func Test_FindEnvironmentNamespaceConflict(t *testing.T) {
	t.Run("reports a conflict from a different resource group", func(t *testing.T) {
		// The regression this guards: the check used to be scoped to a single resource group,
		// so creating an environment in a second group silently reused the namespace.
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, nil, []database.Object{previewEnvironment(envInDefaultGroup, "default")})

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "default", envInOtherGroup)
		require.NoError(t, err)
		require.Equal(t, envInDefaultGroup, conflict)
	})

	t.Run("reports a conflict across environment resource types", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, []database.Object{legacyEnvironment(legacyEnvID, "shared", rpv1.KubernetesComputeKind)}, nil)

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "shared", envInOtherGroup)
		require.NoError(t, err)
		require.Equal(t, legacyEnvID, conflict)
	})

	t.Run("ignores the environment being updated", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, nil, []database.Object{previewEnvironment(envInDefaultGroup, "default")})

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "default", envInDefaultGroup)
		require.NoError(t, err)
		require.Empty(t, conflict)
	})

	t.Run("ignores the environment being updated regardless of ID casing", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, nil, []database.Object{previewEnvironment(envInDefaultGroup, "default")})

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "default", strings.ToUpper(envInDefaultGroup))
		require.NoError(t, err)
		require.Empty(t, conflict)
	})

	t.Run("inspects every match rather than only the first", func(t *testing.T) {
		// A plane-wide query can return several environments. The environment being updated may
		// come back first, which must not mask a genuine conflict later in the list.
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, nil, []database.Object{
			previewEnvironment(envInDefaultGroup, "default"),
			previewEnvironment(envInOtherGroup, "default"),
		})

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "default", envInDefaultGroup)
		require.NoError(t, err)
		require.Equal(t, envInOtherGroup, conflict)
	})

	t.Run("ignores ACI environments", func(t *testing.T) {
		// ACI environments run on Azure Container Instances and occupy no Kubernetes namespace.
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, []database.Object{legacyEnvironment(legacyEnvID, "shared", rpv1.ACIComputeKind)}, nil)

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "shared", envInOtherGroup)
		require.NoError(t, err)
		require.Empty(t, conflict)
	})

	t.Run("returns no conflict when the namespace is free", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))
		expectQueries(client, nil, nil)

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "free", envInOtherGroup)
		require.NoError(t, err)
		require.Empty(t, conflict)
	})

	t.Run("skips the lookup entirely for an empty namespace", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))
		client.EXPECT().Query(gomock.Any(), gomock.Any()).Times(0)

		conflict, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "", envInOtherGroup)
		require.NoError(t, err)
		require.Empty(t, conflict)
	})

	t.Run("queries the whole plane recursively for both types", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))

		queriedTypes := []string{}
		client.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
				require.Equal(t, testPlaneScope, query.RootScope)
				require.True(t, query.ScopeRecursive, "the namespace check must span every resource group in the plane")
				require.Len(t, query.Filters, 1)
				require.Equal(t, "default", query.Filters[0].Value)

				queriedTypes = append(queriedTypes, query.ResourceType)
				return &database.ObjectQueryResult{}, nil
			}).Times(2)

		_, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "default", envInOtherGroup)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{datamodel.EnvironmentResourceType, datamodel.EnvironmentResourceType_v20250801preview}, queriedTypes)
	})

	t.Run("propagates query errors", func(t *testing.T) {
		client := database.NewMockClient(gomock.NewController(t))
		client.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("database unavailable")).
			Times(1)

		_, err := FindEnvironmentNamespaceConflict(t.Context(), client, testPlaneScope, "default", envInOtherGroup)
		require.ErrorContains(t, err, "database unavailable")
	})
}

func Test_NamespaceConflictMessage(t *testing.T) {
	message := NamespaceConflictMessage("default", envInDefaultGroup)
	require.Equal(t, "The Kubernetes namespace specified (default) is already used by another Radius Environment ("+envInDefaultGroup+"). Specify a unique Kubernetes namespace.", message)
}
