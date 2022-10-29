// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
)

func TestListSqlDatabasesRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	_, sqlDataModel, expectedOutput := getTestModels20220315privatepreview()

	t.Run("empty list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSqlDatabases(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.SQLDatabaseList{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, 0, len(actualOutput.Value))
		require.Nil(t, actualOutput.NextLink)
	})

	testCases := []struct {
		description string
		dbCount     int
		batchCount  int
		max         string
		skipToken   bool
	}{
		{"list-sqldb-more-items-than-max", 10, 5, "5", true},
		{"list-sqldb-less-items-than-max", 5, 5, "10", false},
		{"list-sqldb-no-max", 5, 5, "", false},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprint(testCase.description), func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)

			q := req.URL.Query()
			q.Add("top", testCase.max)
			req.URL.RawQuery = q.Encode()

			ctx := radiustesting.ARMTestContextFromRequest(req)

			paginationToken := ""
			if testCase.skipToken {
				paginationToken = "nextLink"
			}

			items := []store.Object{}
			for i := 0; i < testCase.batchCount; i++ {
				item := store.Object{
					Metadata: store.Metadata{
						ID: uuid.New().String(),
					},
					Data: sqlDataModel,
				}
				items = append(items, item)
			}

			mStorageClient.
				EXPECT().
				Query(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
					return &store.ObjectQueryResult{
						Items:           items,
						PaginationToken: paginationToken,
					}, nil
				})

			opts := ctrl.Options{
				StorageClient: mStorageClient,
			}

			ctl, err := NewListSqlDatabases(opts)

			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			actualOutput := &v20220315privatepreview.SQLDatabaseList{}
			_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
			require.Equal(t, testCase.batchCount, len(actualOutput.Value))
			require.Equal(t, expectedOutput, actualOutput.Value[0])

			if testCase.skipToken {
				require.NotNil(t, actualOutput.NextLink)
			} else {
				require.Nil(t, actualOutput.NextLink)
			}
		})
	}
}
