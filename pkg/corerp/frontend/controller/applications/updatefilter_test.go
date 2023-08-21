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
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/test/k8sutil"

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	testAppID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/app0"
	testEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
)

func TestCreateAppScopedNamespace_valid_namespace(t *testing.T) {
	tCtx := rpctest.NewControllerContext(t)

	opts := ctrl.Options{
		StorageClient: tCtx.MockSC,
		DataProvider:  tCtx.MockSP,
		KubeClient:    k8sutil.NewFakeKubeClient(nil),
	}

	t.Run("override namespace", func(t *testing.T) {
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
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
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
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			}).Times(2)

		tCtx.MockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(tCtx.MockSC, nil).Times(1)

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
			Return(rpctest.FakeStoreObject(envdm), nil)

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
		StorageClient: tCtx.MockSC,
		DataProvider:  tCtx.MockSP,
		KubeClient:    k8sutil.NewFakeKubeClient(nil),
	}

	t.Run("invalid namespace", func(t *testing.T) {
		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
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

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{*rpctest.FakeStoreObject(envdm)},
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
			Return(&store.ObjectQueryResult{}, nil).Times(1)
		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&store.ObjectQueryResult{Items: []store.Object{*rpctest.FakeStoreObject(newResource)}}, nil).Times(1)

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

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&store.ObjectQueryResult{}, nil).Times(2)

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
