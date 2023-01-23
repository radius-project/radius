// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

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
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateRedisCacheRun_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm
	}

	/*
		Creating a RedisCache resource in an async way has multiple operations with branching:
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
			"async-create-new-rediscache-success",
			&store.ErrNotFound{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-create-new-rediscache-concurrency-error",
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-create-new-rediscache-enqueue-error",
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

			redisInput, redisDataModel, _ := getTestModels20220315privatepreview()

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderfile, redisInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

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

			opts := frontend_ctrl.Options{
				Options: ctrl.Options{
					StorageClient: mds,
					StatusManager: msm,
				},
			}

			ctl, err := NewCreateOrUpdateRedisCache(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)

			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				locationHeader := getAsyncLocationPath(sCtx, redisDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, redisDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}
}

func getAsyncLocationPath(sCtx *v1.ARMRequestContext, location string, resourceType string, req *http.Request) string {
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
