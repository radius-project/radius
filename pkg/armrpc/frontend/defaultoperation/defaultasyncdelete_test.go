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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestDefaultAsyncDelete(t *testing.T) {
	deleteCases := []struct {
		desc             string
		etag             string
		curState         v1.ProvisioningState
		getErr           error
		qErr             error
		saveErr          error
		rejectedByFilter bool
		code             int
	}{
		{"async-delete-non-existing-resource-no-etag", "", v1.ProvisioningStateNone, &store.ErrNotFound{}, nil, nil, false, http.StatusNoContent},
		{"async-delete-existing-resource-blocked-by-filter", "", v1.ProvisioningStateSucceeded, nil, nil, nil, true, http.StatusConflict},
		{"async-delete-existing-resource-not-in-terminal-state", "", v1.ProvisioningStateUpdating, nil, nil, nil, false, http.StatusConflict},
		{"async-delete-existing-resource-success", "", v1.ProvisioningStateSucceeded, nil, nil, nil, false, http.StatusAccepted},
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

			appDataModel.InternalMetadata.AsyncProvisioningState = tt.curState

			mds.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{
					Metadata: store.Metadata{ID: appDataModel.ID},
					Data:     appDataModel,
				}, tt.getErr).
				Times(1)

			if tt.getErr == nil && !tt.rejectedByFilter && appDataModel.InternalMetadata.AsyncProvisioningState.IsTerminal() {
				msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.qErr).
					Times(1)

				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
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

			ctl, err := NewDefaultAsyncDelete(opts, resourceOpts)
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
