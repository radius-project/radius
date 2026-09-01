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

package environments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/test/k8sutil"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"uuid"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateOrUpdateEnvironmentRun_20231001Preview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	ctx := t.Context()

	createNewResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
		existingNamespace  bool
	}{
		{"create-new-resource-no-if-match", "If-Match", "", "", 200, false, false},
		{"create-new-resource-existing-namespace", "If-Match", "", "", 200, false, true},
		{"create-new-resource-*-if-match", "If-Match", "*", "", 412, true, false},
		{"create-new-resource-etag-if-match", "If-Match", "randome-etag", "", 412, true, false},
		{"create-new-resource-*-if-none-match", "If-None-Match", "*", "", 200, false, false},
	}

	for _, tt := range createNewResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			envInput, envDataModel, expectedOutput := getTestModels20231001preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return nil, &database.ErrNotFound{ID: id}
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
			expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
			expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil),
			}

			if tt.existingNamespace {
				err = opts.KubeClient.Create(ctx, &corev1.Namespace{Name: envDataModel.Properties.Compute.KubernetesCompute.Namespace})
				require.NoError(t, err)
			}
			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
			if !tt.shouldFail {
				err = opts.KubeClient.Get(ctx, client.ObjectKey{Name: envDataModel.Properties.Compute.KubernetesCompute.Namespace}, &corev1.Namespace{})
				require.NoError(t, err)
				actualOutput := &v20231001preview.EnvironmentResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "new-resource-etag", w.Header().Get("ETag"))
			}
		})
	}

	updateExistingResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"update-resource-no-if-match", "If-Match", "", "resource-etag", 200, false},
		{"update-resource-*-if-match", "If-Match", "*", "resource-etag", 200, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "matching-etag", 200, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "another-etag", 412, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "another-etag", 412, true},
	}

	for _, tt := range updateExistingResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			envInput, envDataModel, expectedOutput := getTestModels20231001preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return &database.Object{
						ID: id, ETag: tt.resourceETag,
						Data: envDataModel,
					}, nil
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil),
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)

			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20231001preview.EnvironmentResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "updated-resource-etag", w.Header().Get("ETag"))
			}
		})
	}

	patchNonExistingResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceEtag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"patch-non-existing-resource-no-if-match", "If-Match", "", "", 404, true},
		{"patch-non-existing-resource-*-if-match", "If-Match", "*", "", 404, true},
		{"patch-non-existing-resource-random-if-match", "If-Match", "randome-etag", "", 404, true},
	}

	for _, tt := range patchNonExistingResourceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			envInput, _, _ := getTestModels20231001preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPatch, testHeaderfile, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return nil, &database.ErrNotFound{ID: id}
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil),
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}

	patchExistingResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceEtag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"patch-existing-resource-no-if-match", "If-Match", "", "resource-etag", 200, false},
		{"patch-existing-resource-*-if-match", "If-Match", "*", "resource-etag", 200, false},
		{"patch-existing-resource-matching-if-match", "If-Match", "matching-etag", "matching-etag", 200, false},
		{"patch-existing-resource-not-matching-if-match", "If-Match", "not-matching-etag", "another-etag", 412, true},
	}

	for _, tt := range patchExistingResourceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			envInput, envDataModel, expectedOutput := getTestModels20231001preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPatch, testHeaderfile, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return &database.Object{
						ID: id, ETag: tt.resourceEtag,
						Data: envDataModel,
					}, nil
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						cfg := database.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil),
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20231001preview.EnvironmentResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)
			}
		})
	}

	existingResourceNamespaceCases := []struct {
		desc                 string
		headerKey            string
		headerValue          string
		resourceEtag         string
		existingResourceName string
		expectedStatusCode   int
		shouldFail           bool
	}{
		{"create-existing-namespace-match", "If-Match", "", "resource-etag", "env1", 409, true},
		{"create-existing-namespace-match-same-resource", "If-Match", "", "resource-etag", "env0", 200, false},
	}

	for _, tt := range existingResourceNamespaceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			envInput, envDataModel, _ := getTestModels20231001preview()
			_, conflictDataModel, _ := getTestModels20231001preview()

			conflictDataModel.Name = "existing"
			conflictDataModel.ID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/" + tt.existingResourceName
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPatch, testHeaderfile, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return &database.Object{
						ID: id, ETag: tt.resourceEtag,
						Data: envDataModel,
					}, nil
				})

			paginationToken := "nextLink"

			items := []database.Object{
				{
					ID:   uuid.New().String(),
					Data: conflictDataModel,
				},
			}

			databaseClient.
				EXPECT().
				Query(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
					return &database.ObjectQueryResult{
						Items:           items,
						PaginationToken: paginationToken,
					}, nil
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						cfg := database.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil),
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}
}

// TestCreateOrUpdateEnvironment_NamespaceConflictError verifies the namespace conflict response for
// Applications.Core/environments. Enforcement here is deliberately unchanged -- it stays scoped to
// the current resource group and resource type, because this type is deprecated and widening it
// would break writes on existing installs. Only the error surface improved: a distinct code the
// CLI can recognize, and a message that names the conflicting environment.
func TestCreateOrUpdateEnvironment_NamespaceConflictError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	ctx := t.Context()

	const conflictingEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/existing-env"

	envInput, _, _ := getTestModels20231001preview()
	w := httptest.NewRecorder()
	req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPut, testHeaderfile, envInput)
	require.NoError(t, err)
	ctx = rpctest.NewARMRequestContext(req)

	databaseClient.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return nil, &database.ErrNotFound{ID: id}
		})

	// A different environment in the same resource group already holds the namespace.
	conflicting := &datamodel.Environment{}
	conflicting.ID = conflictingEnvID
	conflicting.Properties.Compute = rpv1.EnvironmentCompute{
		Kind:              rpv1.KubernetesComputeKind,
		KubernetesCompute: rpv1.KubernetesComputeProperties{Namespace: "default"},
	}

	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
			// Enforcement must remain scoped to this resource group and resource type.
			require.False(t, query.ScopeRecursive, "Applications.Core enforcement must not search the whole plane")
			require.Equal(t, "applications.core/environments", strings.ToLower(query.ResourceType))

			return &database.ObjectQueryResult{
				Items: []database.Object{{ID: conflictingEnvID, Data: conflicting}},
			}, nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
		KubeClient:     k8sutil.NewFakeKubeClient(nil),
	}

	ctl, err := NewCreateOrUpdateEnvironment(opts)
	require.NoError(t, err)

	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))
	require.Equal(t, 409, w.Result().StatusCode)

	errorResponse := &v1.ErrorResponse{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), errorResponse))
	require.Equal(t, v1.CodeNamespaceAlreadyInUse, errorResponse.Error.Code)
	require.Contains(t, errorResponse.Error.Message, conflictingEnvID)
}
