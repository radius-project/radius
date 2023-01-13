// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestDeleteMongoDatabase_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *deployment.MockDeploymentProcessor) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, mDeploymentProcessor
	}

	t.Parallel()

	deleteCases := []struct {
		desc       string
		etag       string
		curState   v1.ProvisioningState
		getErr     error
		qErr       error
		saveErr    error
		code       int
		shouldFail bool
	}{
		{"async-delete-non-existing-resource-no-etag", "", v1.ProvisioningStateNone, &store.ErrNotFound{}, nil, nil, http.StatusNoContent, true},
		{"async-delete-existing-resource-not-in-terminal-state", "random-etag", v1.ProvisioningStateUpdating, nil, nil, nil, http.StatusConflict, true},
		{"async-delete-existing-resource-success", "random-etag", v1.ProvisioningStateSucceeded, nil, nil, nil, http.StatusOK, false},
	}

	for _, tt := range deleteCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, mDeploymentProcessor := setupTest(t)
			defer teardownTest(t)

			w := httptest.NewRecorder()

			req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", tt.etag)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			_, appDataModel, _ := getTestModels20220315privatepreview()

			appDataModel.InternalMetadata.AsyncProvisioningState = tt.curState

			mds.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{
					Metadata: store.Metadata{ID: appDataModel.ID, ETag: tt.etag},
					Data:     appDataModel,
				}, tt.getErr).
				Times(1)

			if !tt.shouldFail {
				mDeploymentProcessor.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(nil)
				mds.
					EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
						return nil
					})
			}

			opts := frontend_ctrl.Options{
				Options: ctrl.Options{
					StorageClient: mds,
				},
				DeployProcessor: mDeploymentProcessor,
			}

			ctl, err := NewDeleteMongoDatabase(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)

			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, tt.code, result.StatusCode)

			// If happy path, expect that the returned object has Deleting state
			if tt.code == http.StatusOK {
				actualOutput := &v20220315privatepreview.MongoDatabaseResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
			}
		})
	}
}
