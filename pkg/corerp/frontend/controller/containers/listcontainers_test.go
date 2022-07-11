// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

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

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

func TestListContainersRun_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient) {
		mctrl := gomock.NewController(t)
		mStorageClient := store.NewMockStorageClient(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mStorageClient
	}

	_, dataModel, expectedOutput := getTestModels20220315privatepreview()

	t.Run("list zero resources", func(t *testing.T) {
		teardownTest, msc := setupTest(t)
		defer teardownTest(t)

		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		msc.EXPECT().
			Query(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: msc,
		}

		ctl, err := NewListContainers(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.ContainerResourceList{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, 0, len(actualOutput.Value))
		require.Nil(t, actualOutput.NextLink)
	})

	listCases := []struct {
		desc       string
		dbCount    int
		batchCount int
		top        string
		skipToken  bool
	}{
		{"list-more-items-than-top", 10, 5, "5", true},
		{"list-less-items-than-top", 5, 5, "10", false},
		{"list-no-top", 5, 5, "", false},
	}

	for _, tt := range listCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			teardownTest, msc := setupTest(t)
			defer teardownTest(t)

			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodGet, testHeaderfile, nil)

			q := req.URL.Query()
			q.Add("top", tt.top)
			req.URL.RawQuery = q.Encode()

			ctx := radiustesting.ARMTestContextFromRequest(req)

			paginationToken := ""
			if tt.skipToken {
				paginationToken = "nextLink"
			}

			items := []store.Object{}
			for i := 0; i < tt.batchCount; i++ {
				item := store.Object{
					Metadata: store.Metadata{
						ID: uuid.New().String(),
					},
					Data: dataModel,
				}
				items = append(items, item)
			}

			msc.EXPECT().
				Query(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
					return &store.ObjectQueryResult{
						Items:           items,
						PaginationToken: paginationToken,
					}, nil
				})

			opts := ctrl.Options{
				StorageClient: msc,
			}

			ctl, err := NewListContainers(opts)

			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, http.StatusOK, w.Result().StatusCode)

			actualOutput := &v20220315privatepreview.ContainerResourceList{}
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
