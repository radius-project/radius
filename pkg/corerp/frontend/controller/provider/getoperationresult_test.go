// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestGetOperationResultRun(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	rawDataModel := radiustesting.ReadFixture("operationstatus_datamodel.json")
	osDataModel := &asyncoperation.AsyncOperationStatus{}
	_ = json.Unmarshal(rawDataModel, osDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("operationstatus_output.json")
	expectedOutput := &armrpcv1.AsyncOperationStatus{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	t.Run("get non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		ctl, err := NewGetOperationResult(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	})

	opResTestCases := []struct {
		desc              string
		provisioningState basedatamodel.ProvisioningStates
		respCode          int
		headersCheck      bool
	}{
		{"not-in-terminal-state", basedatamodel.ProvisioningStateAccepted, http.StatusAccepted, true},
		{"succeeded-state", basedatamodel.ProvisioningStateSucceeded, http.StatusOK, true},
		{"failed-state", basedatamodel.ProvisioningStateFailed, http.StatusOK, true},
	}

	for _, tt := range opResTestCases {
		t.Run(tt.desc, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			osDataModel.Status = tt.provisioningState

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id},
						Data:     osDataModel,
					}, nil
				})

			ctl, err := NewGetOperationResult(mStorageClient, nil)

			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.respCode, w.Result().StatusCode)

			if tt.headersCheck {
				require.NotNil(t, w.Result().Header.Get("Location"))
				require.NotNil(t, w.Result().Header.Get("Retry-After"))
			}
		})
	}
}
