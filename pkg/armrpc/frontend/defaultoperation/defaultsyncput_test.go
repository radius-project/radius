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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/ucp/database"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDefaultSyncPut_Create(t *testing.T) {
	createCases := []struct {
		desc    string
		getErr  error
		saveErr error
		rCode   int
		rErr    error
	}{
		{
			"sync-create-new-resource-success",
			&database.ErrNotFound{},
			nil,
			http.StatusOK,
			nil,
		},
		{
			"sync-create-new-resource-concurrency-error",
			&database.ErrConcurrency{},
			nil,
			http.StatusOK,
			&database.ErrConcurrency{},
		},
	}

	for _, tt := range createCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			reqModel, _, _ := loadTestResurce()

			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(context.Background(), http.MethodPut, resourceTestHeaderFile, reqModel)
			require.NoError(t, err)

			ctx := rpctest.NewARMRequestContext(req)

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(&database.Object{}, tt.getErr).
				Times(1)

			if tt.getErr == nil || errors.Is(&database.ErrNotFound{}, tt.getErr) {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)
			}

			opts := ctrl.Options{
				DatabaseClient: mds,
				StatusManager:  msm,
			}

			resourceOpts := ctrl.ResourceOptions[TestResourceDataModel]{
				RequestConverter:  testResourceDataModelFromVersioned,
				ResponseConverter: testResourceDataModelToVersioned,
				UpdateFilters: []ctrl.UpdateFilter[TestResourceDataModel]{
					testValidateRequest,
				},
			}

			ctl, err := NewDefaultSyncPut(opts, resourceOpts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)

			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)
			}
		})
	}
}

func TestDefaultSyncPut_Update(t *testing.T) {
	updateCases := []struct {
		desc               string
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
			"sync-update-existing-resource-success",
			"resource-sync-request.json",
			"resource-sync-datamodel.json",
			nil,
			false,
			nil,
			nil,
			nil,
			http.StatusOK,
			nil,
		},
		{
			"sync-update-existing-resource-invalid-request",
			"resource-sync-request-invalid.json",
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
			"sync-update-existing-resource-concurrency-error",
			"resource-request.json",
			"resource-datamodel.json",
			nil,
			false,
			&database.ErrConcurrency{},
			nil,
			nil,
			http.StatusInternalServerError,
			&database.ErrConcurrency{},
		},
		{
			"sync-update-existing-resource-save-error",
			"resource-request.json",
			"resource-datamodel.json",
			nil,
			false,
			&database.ErrInvalid{Message: "testing initial save err"},
			nil,
			nil,
			http.StatusInternalServerError,
			&database.ErrInvalid{Message: "testing initial save err"},
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

			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(context.Background(), http.MethodPatch, resourceTestHeaderFile, reqModel)
			require.NoError(t, err)

			ctx := rpctest.NewARMRequestContext(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

			so := &database.Object{
				Metadata: database.Metadata{ID: sCtx.ResourceID.String()},
				Data:     reqDataModel,
			}

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(so, tt.getErr).
				Times(1)

			if tt.getErr == nil && !tt.skipSave {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)
			}

			opts := ctrl.Options{
				DatabaseClient: mds,
				StatusManager:  msm,
			}

			resourceOpts := ctrl.ResourceOptions[TestResourceDataModel]{
				RequestConverter:  testResourceDataModelFromVersioned,
				ResponseConverter: testResourceDataModelToVersioned,
				UpdateFilters: []ctrl.UpdateFilter[TestResourceDataModel]{
					testValidateRequest,
				},
			}

			ctl, err := NewDefaultSyncPut(opts, resourceOpts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			if resp != nil {
				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)
			}

			if tt.rCode == http.StatusAccepted {
				require.NoError(t, err)
			}

			if tt.rErr != nil {
				require.ErrorIs(t, tt.rErr, err)
			}
		})
	}
}
