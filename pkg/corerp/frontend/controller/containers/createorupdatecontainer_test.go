// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateContainerRun_20220315PrivatePreview(t *testing.T) {

	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm
	}

	/*
		Creating a container resource in an async way has multiple operations with branching:
		1. Get Resource
		2. [Conditional] If resource exists, check if there is an ongoing operation on it
		3. Save Resource
		4. Queue Resource
		5. [Conditional] If Queue has an error then Rollback changes
		6. [Conditional] Update the record state to Failed
	*/
	createCases := []struct {
		desc    string
		getErr  error
		saveErr error
		qErr    error
		rbErr   error
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
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			containerInput, containerDataModel, _ := getTestModels20220315privatepreview()

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderfile, containerInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			sCtx := servicecontext.ARMRequestContextFromContext(ctx)

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
						mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(tt.rbErr).
							Times(1)
					}
				}
			}

			opts := ctrl.Options{
				StorageClient:  mds,
				AsyncOperation: msm,
			}

			ctl, err := NewCreateOrUpdateContainer(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)
			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				locationHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
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
			http.StatusAccepted,
			nil,
		},
		{
			"async-update-existing-container-concurrency-error",
			v1.ProvisioningStateSucceeded,
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusAccepted,
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
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			containerInput, containerDataModel, _ := getTestModels20220315privatepreview()
			containerDataModel.Properties.ProvisioningState = tt.curState

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPatch, testHeaderfile, containerInput)
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

			opts := ctrl.Options{
				StorageClient:  mds,
				AsyncOperation: msm,
			}

			ctl, err := NewCreateOrUpdateContainer(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)
			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				locationHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}
}

func getAsyncLocationPath(sCtx *servicecontext.ARMRequestContext, location string, resourceType string, req *http.Request) string {
	dest := url.URL{
		Host:   req.Host,
		Scheme: req.URL.Scheme,
		Path: fmt.Sprintf("%s/providers/%s/locations/%s/%s/%s", sCtx.ResourceID.PlaneScope(),
			sCtx.ResourceID.ProviderNamespace(), location, resourceType, sCtx.OperationID.String()),
	}

	query := url.Values{}
	query.Add("api-version", sCtx.APIVersion)
	dest.RawQuery = query.Encode()

	// In production this is the header we get from app service for the 'real' protocol
	protocol := req.Header.Get("X-Forwarded-Proto")
	if protocol != "" {
		dest.Scheme = protocol
	}

	if dest.Scheme == "" {
		dest.Scheme = "http"
	}

	return dest.String()
}
