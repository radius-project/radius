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

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

type testDataModel struct {
	Name string `json:"name"`
}

func (e testDataModel) ResourceTypeName() string {
	return "Applications.Test/resource"
}

func (e testDataModel) GetSystemData() *v1.SystemData {
	return nil
}

func (e testDataModel) GetBaseResource() *v1.BaseResource {
	return nil
}

func (e testDataModel) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningStateAccepted
}

func (e testDataModel) SetProvisioningState(state v1.ProvisioningState) {
}

func (e testDataModel) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {
}

type testVersionedModel struct {
	Name string `json:"name"`
}

func (v *testVersionedModel) ConvertFrom(src v1.DataModelInterface) error {
	dm := src.(*testDataModel)
	v.Name = dm.Name
	return nil
}

func (v *testVersionedModel) ConvertTo() (v1.DataModelInterface, error) {
	return nil, nil
}

func resourceToVersioned(model *testDataModel, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case testAPIVersion:
		versioned := &testVersionedModel{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

func TestGetResourceRun(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	testResourceDataModel := &testDataModel{
		Name: "ResourceName",
	}
	expectedOutput := &testVersionedModel{
		Name: "ResourceName",
	}

	t.Run("get non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, resourceTestHeaderFile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctrlOpts := ctrl.ResourceOptions[testDataModel]{
			ResponseConverter: resourceToVersioned,
		}

		ctl, err := NewGetResource(opts, ctrlOpts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("get existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, resourceTestHeaderFile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     testResourceDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctrlOpts := ctrl.ResourceOptions[testDataModel]{
			ResponseConverter: resourceToVersioned,
		}

		ctl, err := NewGetResource(opts, ctrlOpts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &testVersionedModel{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedOutput, actualOutput)
	})
}
