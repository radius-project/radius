// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

func TestListEnvironmentRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	_, envDataModel, expectedOutput := getTestModels20220315privatepreview()

	t.Run("list zero resources", func(t *testing.T) {
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

		ctl, err := NewListEnvironments(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.EnvironmentResourceList{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, 0, len(actualOutput.Value))
	})

	t.Run("list resources", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{
						{
							Metadata: store.Metadata{ID: envDataModel.ID},
							Data:     envDataModel,
						},
					},
				}, nil
			})

		ctl, err := NewListEnvironments(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.EnvironmentResourceList{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, 1, len(actualOutput.Value))
		require.Equal(t, expectedOutput, actualOutput.Value[0])
	})
}
