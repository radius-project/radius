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

package applications

import (
	"context"
	"strings"
	"testing"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/k8sutil"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testAppID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/app0"
	testEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
)

func TestCreateAppScopedNamespace_valid_namespace(t *testing.T) {
	tCtx := rpctest.NewControllerContext(t)

	opts := ctrl.Options{
		DatabaseClient: tCtx.MockSC,
		KubeClient:     k8sutil.NewFakeKubeClient(nil),
	}

	t.Run("override namespace", func(t *testing.T) {
		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
					KubernetesCompute: rpv1.KubernetesComputeProperties{
						Namespace: "default",
					},
				},
			},
		}

		old := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
					Status: rpv1.ResourceStatus{
						Compute: &rpv1.EnvironmentCompute{
							Kind: rpv1.KubernetesComputeKind,
							KubernetesCompute: rpv1.KubernetesComputeProperties{
								Namespace: "app-ns",
							},
						},
					},
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil)

		tCtx.MockSC.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
				return &database.ObjectQueryResult{
					Items: []database.Object{},
				}, nil
			}).Times(2)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "app-ns"},
					},
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, old, &opts)
		require.NoError(t, err)
		require.Nil(t, resp)

		require.Equal(t, "app-ns", newResource.Properties.Status.Compute.KubernetesCompute.Namespace)
	})

	t.Run("generate namespace with environment", func(t *testing.T) {
		tCtx.MockSC.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
				return &database.ObjectQueryResult{
					Items: []database.Object{},
				}, nil
			}).Times(2)

		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
					KubernetesCompute: rpv1.KubernetesComputeProperties{
						Namespace: "default",
					},
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil).Times(2)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)
		require.NoError(t, err)
		require.Nil(t, resp)

		require.Equal(t, "default-app0", newResource.Properties.Status.Compute.KubernetesCompute.Namespace)
	})
}

func TestCreateAppScopedNamespace_invalid_property(t *testing.T) {
	tCtx := rpctest.NewControllerContext(t)

	opts := ctrl.Options{
		DatabaseClient: tCtx.MockSC,
		KubeClient:     k8sutil.NewFakeKubeClient(nil),
	}

	t.Run("generated namespace is invalid", func(t *testing.T) {
		longAppID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/this-is-a-very-long-application-name-that-is-invalid"
		longEnvID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/this-is-a-very-long-environment-name-that-is-invalid"

		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil).Times(2)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: longEnvID,
				},
			},
		}

		id, err := resources.ParseResource(longAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)
		require.NoError(t, err)
		res := resp.(*rest.BadRequestResponse)

		require.Equal(t, "Application namespace 'this-is-a-very-long-environment-name-that-is-invalid-this-is-a-very-long-application-name-that-is-invalid' could not be created: the combination of application and environment names is too long.", res.Body.Error.Message)
	})

	t.Run("invalid namespace", func(t *testing.T) {
		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil)

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
				return &database.ObjectQueryResult{
					Items: []database.Object{},
				}, nil
			}).Times(2)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: strings.Repeat("invalid-name", 6)},
					},
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)
		require.NoError(t, err)
		res := resp.(*rest.BadRequestResponse)
		require.Equal(t, res.Body.Error.Message, "'invalid-nameinvalid-nameinvalid-nameinvalid-nameinvalid-nameinvalid-name' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'. Please specify a valid namespace using 'kubernetesNamespace' extension in '$.properties.extensions[*]'.")
	})

	t.Run("conflicted namespace in environment resource", func(t *testing.T) {
		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
					KubernetesCompute: rpv1.KubernetesComputeProperties{
						Namespace: "testns",
					},
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil)

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
				return &database.ObjectQueryResult{
					Items: []database.Object{*rpctest.FakeStoreObject(envdm)},
				}, nil
			}).Times(1)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "testns"},
					},
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)
		require.NoError(t, err)
		res := resp.(*rest.ConflictResponse)
		require.Equal(t, res.Body.Error.Message, "Environment env0 with the same namespace (testns) already exists")
	})

	t.Run("conflicted namespace in the different application resource", func(t *testing.T) {
		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil)

		newResource := &datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: testAppID,
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "testns"},
					},
				},
			},
		}

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&database.ObjectQueryResult{}, nil).Times(1)
		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&database.ObjectQueryResult{Items: []database.Object{*rpctest.FakeStoreObject(newResource)}}, nil).Times(1)

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)
		require.NoError(t, err)
		res := resp.(*rest.ConflictResponse)
		require.Equal(t, res.Body.Error.Message, "Application /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/app0 with the same namespace (testns) already exists")
	})

	t.Run("update application with the different namespace", func(t *testing.T) {
		old := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
					Status: rpv1.ResourceStatus{
						Compute: &rpv1.EnvironmentCompute{
							Kind: rpv1.KubernetesComputeKind,
							KubernetesCompute: rpv1.KubernetesComputeProperties{
								Namespace: "default-app0",
							},
						},
					},
				},
			},
		}

		newResource := &datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: testAppID,
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "differentname"},
					},
				},
			},
		}

		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil)

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&database.ObjectQueryResult{}, nil).Times(2)

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := CreateAppScopedNamespace(ctx, newResource, old, &opts)
		require.NoError(t, err)
		res := resp.(*rest.BadRequestResponse)
		require.Equal(t, res.Body.Error.Message, "Updating an application's Kubernetes namespace from 'default-app0' to 'differentname' requires the application to be deleted and redeployed. Please delete your application and try again.")
	})
}

func TestCreateAppScopedNamespace_non_kubernetes_compute(t *testing.T) {
	tCtx := rpctest.NewControllerContext(t)

	opts := ctrl.Options{
		DatabaseClient: tCtx.MockSC,
		KubeClient:     k8sutil.NewFakeKubeClient(nil),
	}

	t.Run("skips namespace creation for ACI compute", func(t *testing.T) {
		// Set up environment with ACI compute
		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.ACIComputeKind,
				},
			},
		}

		// Expect to get the environment but no other calls
		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(rpctest.FakeStoreObject(envdm), nil)

		// Create application resource
		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		// Call the function
		resp, err := CreateAppScopedNamespace(ctx, newResource, nil, &opts)

		// Verify it returns nil, nil and doesn't modify the resource
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Nil(t, newResource.Properties.Status.Compute, "Compute status should not be set for ACI compute kind")
	})
}
