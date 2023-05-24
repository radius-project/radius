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

package containers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestDeleteContainerRun_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm
	}

	t.Parallel()

	deleteCases := []struct {
		desc     string
		etag     string
		curState v1.ProvisioningState
		getErr   error
		qErr     error
		saveErr  error
		code     int
	}{
		{"async-delete-non-existing-resource-no-etag", "", v1.ProvisioningStateNone, &store.ErrNotFound{}, nil, nil, http.StatusNoContent},
		{"async-delete-existing-resource-not-in-terminal-state", "", v1.ProvisioningStateUpdating, nil, nil, nil, http.StatusConflict},
		{"async-delete-existing-resource-success", "", v1.ProvisioningStateSucceeded, nil, nil, nil, http.StatusAccepted},
	}

	for _, tt := range deleteCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			w := httptest.NewRecorder()

			req, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", tt.etag)

			ctx := testutil.ARMTestContextFromRequest(req)
			_, appDataModel, _ := getTestModels20220315privatepreview()

			appDataModel.InternalMetadata.AsyncProvisioningState = tt.curState

			mds.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{
					Metadata: store.Metadata{ID: appDataModel.ID},
					Data:     appDataModel,
				}, tt.getErr).
				Times(1)

			if tt.getErr == nil && appDataModel.InternalMetadata.AsyncProvisioningState.IsTerminal() {
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

			ctl, err := NewDeleteContainer(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)

			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, tt.code, result.StatusCode)

			// If happy path, expect that the returned object has Deleting state
			if tt.code == http.StatusAccepted {
				actualOutput := &v20220315privatepreview.ContainerResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
			}
		})
	}
}
