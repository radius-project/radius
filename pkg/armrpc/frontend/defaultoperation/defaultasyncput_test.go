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

package defaultoperation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestDefaultAsyncPut_Create(t *testing.T) {
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
			"async-create-new-resource-success",
			&store.ErrNotFound{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-create-new-resource-concurrency-error",
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-create-new-resource-enqueue-error",
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

			reqModel, reqDataModel, _ := loadTestResurce()

			w := httptest.NewRecorder()
			req, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, resourceTestHeaderFile, reqModel)
			require.NoError(t, err)

			ctx := testutil.ARMTestContextFromRequest(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

			// These values don't affect the test since we're using mocks. Just choosing non-default values
			// to verify that they're being passed through.
			var asyncOperationTimeout = 1*time.Second + 1*time.Millisecond
			var asyncOperationRetryAfter = 2*time.Second + 2*time.Millisecond

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{}, tt.getErr).
				Times(1)

			if tt.getErr == nil || errors.Is(&store.ErrNotFound{}, tt.getErr) {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)

				if tt.saveErr == nil {
					expectedOptions := statusmanager.QueueOperationOptions{
						OperationTimeout: asyncOperationTimeout,
						RetryAfter:       asyncOperationRetryAfter,
					}
					msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), expectedOptions).
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
				StorageClient: mds,
				StatusManager: msm,
			}

			resourceOpts := ctrl.ResourceOptions[TestResourceDataModel]{
				RequestConverter:  testResourceDataModelFromVersioned,
				ResponseConverter: testResourceDataModelToVersioned,
				UpdateFilters: []ctrl.UpdateFilter[TestResourceDataModel]{
					testValidateRequest,
				},
				AsyncOperationTimeout:    asyncOperationTimeout,
				AsyncOperationRetryAfter: asyncOperationRetryAfter,
			}

			ctl, err := NewDefaultAsyncPut(opts, resourceOpts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)

			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				locationHeader := getAsyncLocationPath(sCtx, reqDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, reqDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))

				expectedRetryAfterHeader := "2"
				require.NotNil(t, w.Header().Get("Retry-After"))
				require.Equal(t, expectedRetryAfterHeader, w.Header().Get("Retry-After"))
			}
		})
	}
}

func TestDefaultAsyncPut_Update(t *testing.T) {
	updateCases := []struct {
		desc               string
		curState           v1.ProvisioningState
		versionedInputFile string
		datamodelFile      string
		getErr             error
		skipSave           bool
		saveErr            error
		qErr               error
		rbErr              error
		rCode              int
		rErr               error
	}{
		{
			"async-update-existing-resource-success",
			v1.ProvisioningStateSucceeded,
			"resource-request.json",
			"resource-datamodel.json",
			nil,
			false,
			nil,
			nil,
			nil,
			http.StatusAccepted,
			nil,
		},
		{
			"async-update-existing-resource-mismatched-appid",
			v1.ProvisioningStateSucceeded,
			"resource-request-invalidapp.json",
			"resource-datamodel.json",
			nil,
			true,
			nil,
			nil,
			nil,
			http.StatusBadRequest,
			nil,
		},
		{
			"async-update-existing-resource-concurrency-error",
			v1.ProvisioningStateSucceeded,
			"resource-request.json",
			"resource-datamodel.json",
			nil,
			false,
			&store.ErrConcurrency{},
			nil,
			nil,
			http.StatusInternalServerError,
			&store.ErrConcurrency{},
		},
		{
			"async-update-existing-resource-save-error",
			v1.ProvisioningStateSucceeded,
			"resource-request.json",
			"resource-datamodel.json",
			nil,
			false,
			&store.ErrInvalid{Message: "testing initial save err"},
			nil,
			nil,
			http.StatusInternalServerError,
			&store.ErrInvalid{Message: "testing initial save err"},
		},
		{
			"async-update-existing-resource-enqueue-error",
			v1.ProvisioningStateSucceeded,
			"resource-request.json",
			"resource-datamodel.json",
			nil,
			false,
			nil,
			&store.ErrInvalid{Message: "testing initial save err"},
			nil,
			http.StatusInternalServerError,
			&store.ErrInvalid{Message: "testing initial save err"},
		},
	}

	for _, tt := range updateCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			reqModel := &TestResource{}
			_ = json.Unmarshal(testutil.ReadFixture(tt.versionedInputFile), reqModel)

			reqDataModel := &TestResourceDataModel{}
			_ = json.Unmarshal(testutil.ReadFixture(tt.datamodelFile), reqDataModel)

			reqDataModel.InternalMetadata.AsyncProvisioningState = tt.curState

			w := httptest.NewRecorder()
			req, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPatch, resourceTestHeaderFile, reqModel)
			require.NoError(t, err)

			ctx := testutil.ARMTestContextFromRequest(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

			so := &store.Object{
				Metadata: store.Metadata{ID: sCtx.ResourceID.String()},
				Data:     reqDataModel,
			}

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(so, tt.getErr).
				Times(1)

			if tt.getErr == nil && !tt.skipSave {
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
				StorageClient: mds,
				StatusManager: msm,
			}

			resourceOpts := ctrl.ResourceOptions[TestResourceDataModel]{
				RequestConverter:  testResourceDataModelFromVersioned,
				ResponseConverter: testResourceDataModelToVersioned,
				UpdateFilters: []ctrl.UpdateFilter[TestResourceDataModel]{
					testValidateRequest,
				},
			}

			ctl, err := NewDefaultAsyncPut(opts, resourceOpts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			if resp != nil {
				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)
			}

			if tt.rCode == http.StatusAccepted {
				require.NoError(t, err)

				locationHeader := getAsyncLocationPath(sCtx, reqDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, reqDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
			}

			if tt.rErr != nil {
				require.ErrorIs(t, tt.rErr, err)
			}
		})
	}
}
