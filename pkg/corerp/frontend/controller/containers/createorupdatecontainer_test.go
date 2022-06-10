// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateContainerRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mds := store.NewMockStorageClient(mctrl)
	msm := statusmanager.NewMockStatusManager(mctrl)

	ctx := context.Background()

	/*
		Creating a container resource in an async way has multiple operations with branching:
		1. Get Resource
		2. [Conditional] If resource exists, check if there is an ongoing operation on it
		3. Save Resource
		4. Queue Resource
		5. [Conditional] If Queue has an error then Rollback changes
		6. [Conditional] If resource already existed, write the old copy back
		7. [Conditional] If resource didn't exist, delete the newly created record
	*/
	createCases := []struct {
		desc    string
		getErr  error
		saveErr error
		qErr    error
		delErr  error
		rCode   int
		rErr    error
	}{
		{
			"async-create-new-container-success",
			&store.ErrNotFound{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-create-new-container-concurrency-error",
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-create-new-container-enqueue-error",
			&store.ErrNotFound{},
			nil,
			errors.New("enqueuer client is unset"),
			nil,
			http.StatusInternalServerError,
			errors.New("enqueuer client is unset"),
		},
	}

	for _, tt := range createCases {
		t.Run(tt.desc, func(t *testing.T) {
			containerInput, _, _ := getTestModels20220315privatepreview()

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPut, testHeaderfile, containerInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			// sCtx := servicecontext.ARMRequestContextFromContext(ctx)

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{}, tt.getErr).
				Times(1)

			if tt.getErr == nil || errors.Is(&store.ErrNotFound{}, tt.getErr) {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)

				if tt.saveErr == nil {
					msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(tt.qErr).
						Times(1)

					if tt.qErr != nil {
						mds.EXPECT().Delete(gomock.Any(), gomock.Any()).
							Return(tt.delErr).
							Times(1)
					}
				}
			}

			ctl, err := NewCreateOrUpdateContainer(mds, msm)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)
			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				// require.NotNil(t, w.Header().Get("Location"))
				// require.Equal(t, GetOperationResultPath(req, sCtx.OperationID.String()),
				// 	w.Header().Get("Location"))

				// FIXME: "/subscriptions//providers//locations//operationsStatuses/135dd863-6c0a-4677-8429-b99b5e324f99"
				// require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				// require.Equal(t, GetOperationStatusPath(req, sCtx.OperationID.String()),
				// 	w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}

	updateCases := []struct {
		desc     string
		curState v1.ProvisioningState
		getErr   error
		saveErr  error
		qErr     error
		rbErr    error
		rCode    int
		rErr     error
	}{
		{
			"async-update-existing-container-success",
			v1.ProvisioningStateSucceeded,
			nil,
			nil,
			nil,
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-update-existing-container-concurrency-error",
			v1.ProvisioningStateSucceeded,
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-update-existing-container-save-error",
			v1.ProvisioningStateSucceeded,
			nil,
			errors.New("testing initial save err"),
			nil,
			nil,
			http.StatusInternalServerError,
			errors.New("testing initial save err"),
		},
		{
			"async-update-existing-container-enqueue-error",
			v1.ProvisioningStateSucceeded,
			nil,
			nil,
			errors.New("enqueuer client is unset"),
			nil,
			http.StatusInternalServerError,
			errors.New("enqueuer client is unset"),
		},
	}

	for _, tt := range updateCases {
		t.Run(tt.desc, func(t *testing.T) {
			containerInput, containerDataModel, _ := getTestModels20220315privatepreview()
			containerDataModel.Properties.ProvisioningState = tt.curState

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPut, testHeaderfile, containerInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			sCtx := servicecontext.ARMRequestContextFromContext(ctx)

			so := &store.Object{
				Metadata: store.Metadata{ID: sCtx.ResourceID.String()},
				Data:     containerDataModel,
			}

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(so, tt.getErr).
				Times(1)

			if tt.getErr == nil {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)

				if tt.saveErr == nil {
					msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(tt.qErr).
						Times(1)

					if tt.qErr != nil {
						mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(tt.rbErr).
							Times(1)
					}
				}
			}

			ctl, err := NewCreateOrUpdateContainer(mds, msm)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)
			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				// require.NotNil(t, w.Header().Get("Location"))
				// require.Equal(t, GetOperationResultPath(req, sCtx.OperationID.String()),
				// 	w.Header().Get("Location"))

				// FIXME: "/subscriptions//providers//locations//operationsStatuses/135dd863-6c0a-4677-8429-b99b5e324f99"
				// require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				// require.Equal(t, GetOperationStatusPath(req, sCtx.OperationID.String()),
				// 	w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}
}
