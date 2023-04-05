// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestDefaultSyncDelete(t *testing.T) {
	deleteCases := []struct {
		desc             string
		etag             string
		getErr           error
		deleteErr        error
		rejectedByFilter bool
		code             int
	}{
		{"sync-delete-existing-resource-success", "", nil, nil, false, http.StatusOK},
		{"sync-delete-non-existing-resource", "", &store.ErrNotFound{}, nil, false, http.StatusNoContent},
		{"sync-delete-existing-resource-blocked-by-filter", "", nil, nil, true, http.StatusConflict},
		{"sync-delete-fails-resource-notfound", "", nil, &store.ErrNotFound{}, false, http.StatusNoContent},
	}

	for _, tt := range deleteCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			w := httptest.NewRecorder()

			req, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, resourceTestHeaderFile, nil)
			req.Header.Set("If-Match", tt.etag)

			ctx := testutil.ARMTestContextFromRequest(req)
			_, appDataModel, _ := loadTestResurce()

			mds.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{
					Metadata: store.Metadata{ID: appDataModel.ID},
					Data:     appDataModel,
				}, tt.getErr).
				Times(1)

			if tt.getErr == nil && !tt.rejectedByFilter {

				mds.EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					Return(tt.deleteErr).
					Times(1)
			}

			opts := ctrl.Options{
				StorageClient: mds,
				StatusManager: msm,
			}

			resourceOpts := ctrl.ResourceOptions[TestResourceDataModel]{
				RequestConverter:  testResourceDataModelFromVersioned,
				ResponseConverter: testResourceDataModelToVersioned,
			}

			if tt.rejectedByFilter {
				resourceOpts.DeleteFilters = []ctrl.DeleteFilter[TestResourceDataModel]{
					func(ctx context.Context, oldResource *TestResourceDataModel, options *ctrl.Options) (rest.Response, error) {
						return rest.NewConflictResponse("no way!"), nil
					},
				}
			}

			ctl, err := NewDefaultSyncDelete(opts, resourceOpts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)

			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, tt.code, result.StatusCode)

			// If happy path, expect that the returned object has Accepted state
			if tt.code == http.StatusAccepted {
				actualOutput := &TestResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
			}
		})
	}
}
