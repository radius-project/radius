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

package defaultoperation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type testResourceList struct {
	NextLink *string               `json:"nextLink,omitempty"`
	Value    []*testVersionedModel `json:"value,omitempty"`
}

func TestListResourcesRun(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	ctx := context.Background()

	testResourceDataModel := &testDataModel{
		Name: "ResourceName",
	}
	expectedOutput := &testVersionedModel{
		Name: "ResourceName",
	}

	t.Run("list zero resources", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, resourceTestHeaderFile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)

		databaseClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
				return &database.ObjectQueryResult{
					Items: []database.Object{},
				}, nil
			})

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		ctrlOpts := ctrl.ResourceOptions[testDataModel]{
			ResponseConverter: resourceToVersioned,
		}

		ctl, err := NewListResources(opts, ctrlOpts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)

		actualOutput := &testResourceList{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, 0, len(actualOutput.Value))
		require.Nil(t, actualOutput.NextLink)
	})

	listEnvsCases := []struct {
		desc       string
		headerFile string
		dbCount    int
		batchCount int
		top        string
		skipToken  bool
		planeScope bool
	}{
		{"list-envs-more-items-than-top", resourceTestHeaderFile, 10, 5, "5", true, false},
		{"list-envs-less-items-than-top", resourceTestHeaderFile, 5, 5, "10", false, false},
		{"list-envs-no-top", resourceTestHeaderFile, 5, 5, "", false, false},
		{"list-envs-plane-scope-more-items-than-top", "resource_planescope_requestheaders.json", 5, 5, "", false, false},
	}

	for _, tt := range listEnvsCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, resourceTestHeaderFile, nil)
			require.NoError(t, err)

			q := req.URL.Query()
			q.Add("top", tt.top)
			req.URL.RawQuery = q.Encode()

			ctx := rpctest.NewARMRequestContext(req)
			serviceCtx := v1.ARMRequestContextFromContext(ctx)

			paginationToken := ""
			if tt.skipToken {
				paginationToken = "nextLink"
			}

			items := []database.Object{}
			for i := 0; i < tt.batchCount; i++ {
				item := database.Object{
					Metadata: database.Metadata{
						ID: uuid.New().String(),
					},
					Data: testResourceDataModel,
				}
				items = append(items, item)
			}

			expectedQuery := database.Query{
				RootScope:    serviceCtx.ResourceID.RootScope(),
				ResourceType: serviceCtx.ResourceID.Type(),

				// Most of our tests cases are for the case where the resource scope matches the query
				// scope. eg: environment is scoped to resource groups and the URL of the test request
				// matches the resource group scope.
				ScopeRecursive: tt.planeScope,
			}

			databaseClient.
				EXPECT().
				Query(gomock.Any(), expectedQuery, gomock.Any()).
				DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
					return &database.ObjectQueryResult{
						Items:           items,
						PaginationToken: paginationToken,
					}, nil
				})

			opts := ctrl.Options{
				DatabaseClient: databaseClient,
			}

			ctrlOpts := ctrl.ResourceOptions[testDataModel]{
				ResponseConverter:  resourceToVersioned,
				ListRecursiveQuery: tt.planeScope,
			}

			ctl, err := NewListResources(opts, ctrlOpts)

			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			actualOutput := &testResourceList{}
			_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
			require.Equal(t, tt.batchCount, len(actualOutput.Value))
			require.Equal(t, expectedOutput, actualOutput.Value[0])

			if tt.skipToken {
				require.NotNil(t, actualOutput.NextLink)
			} else {
				require.Nil(t, actualOutput.NextLink)
			}
		})
	}
}
