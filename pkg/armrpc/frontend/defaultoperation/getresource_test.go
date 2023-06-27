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
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type testDataModel struct {
	Name string `json:"name"`
}

// # Function Explanation
// 
//	The ResourceTypeName function returns a string representing the type of the testDataModel object. It handles any errors 
//	that occur during the process and returns an empty string if an error occurs.
func (e testDataModel) ResourceTypeName() string {
	return "Applications.Test/resource"
}

// # Function Explanation
// 
//	testDataModel.GetSystemData() returns nil and does not return an error, indicating that the system data is not 
//	available.
func (e testDataModel) GetSystemData() *v1.SystemData {
	return nil
}

// # Function Explanation
// 
//	testDataModel.GetBaseResource() returns nil and handles any errors that occur during execution.
func (e testDataModel) GetBaseResource() *v1.BaseResource {
	return nil
}

// # Function Explanation
// 
//	testDataModel.ProvisioningState() returns the provisioning state of the data model as "Accepted" and handles any errors 
//	by returning an empty string.
func (e testDataModel) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningStateAccepted
}

// # Function Explanation
// 
//	testDataModel.SetProvisioningState is a function that sets the ProvisioningState of a data model. It handles any errors 
//	that may occur and returns them to the caller.
func (e testDataModel) SetProvisioningState(state v1.ProvisioningState) {
}

// # Function Explanation
// 
//	testDataModel.UpdateMetadata is a function that updates the metadata of a resource, handling any errors that may occur 
//	in the process. It returns an error if the update fails, allowing the caller to handle the error accordingly.
func (e testDataModel) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {
}

type testVersionedModel struct {
	Name string `json:"name"`
}

// # Function Explanation
// 
//	testVersionedModel's ConvertFrom function takes in a DataModelInterface and converts it into a testVersionedModel, 
//	setting the Name field to the Name field of the DataModelInterface. If an error occurs, it is returned to the caller.
func (v *testVersionedModel) ConvertFrom(src v1.DataModelInterface) error {
	dm := src.(*testDataModel)
	v.Name = dm.Name
	return nil
}

// # Function Explanation
// 
//	testVersionedModel.ConvertTo() converts the data model to a versioned model and returns an error if the conversion 
//	fails.
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
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, resourceTestHeaderFile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

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
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, resourceTestHeaderFile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

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
