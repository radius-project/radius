// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprinvokehttproutes"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func getDeploymentProcessorOutputs() (renderers.RendererOutput, deployment.DeploymentOutput) {
	rendererOutput := renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"appId": {
				Value: "daprAppId",
			},
		},
	}

	deploymentOutput := deployment.DeploymentOutput{
		Resources: []outputresource.OutputResource{},
	}

	return rendererOutput, deploymentOutput
}

func TestCreateOrUpdateDaprInvokeHttpRoute_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	rendererOutput, deploymentOutput := getDeploymentProcessorOutputs()
	ctx := context.Background()

	createNewResourceTestCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"create-new-resource-no-if-match", "If-Match", "", "", http.StatusOK, false},
		{"create-new-resource-*-if-match", "If-Match", "*", "", http.StatusPreconditionFailed, true},
		{"create-new-resource-etag-if-match", "If-Match", "random-etag", "", http.StatusPreconditionFailed, true},
		{"create-new-resource-*-if-none-match", "If-None-Match", "*", "", http.StatusOK, false},
	}

	for _, testcase := range createNewResourceTestCases {
		t.Run(testcase.desc, func(t *testing.T) {
			input, dataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, input)
			req.Header.Set(testcase.headerKey, testcase.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
			expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
			expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

			if !testcase.shouldFail {
				deploymentOutput.RadiusResource = dataModel
				deploymentOutput.RadiusResource.(*datamodel.DaprInvokeHttpRoute).Properties.AppId = rendererOutput.ComputedValues[daprinvokehttproutes.AppIDKey].Value.(string)
				mDeploymentProcessor.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(rendererOutput, nil)
				mDeploymentProcessor.EXPECT().Deploy(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(deploymentOutput, nil)

				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = dataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				GetDeploymentProcessor: func() deployment.DeploymentProcessor {
					return mDeploymentProcessor
				},
			}

			ctl, err := NewCreateOrUpdateDaprInvokeHttpRoute(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

			if !testcase.shouldFail {
				actualOutput := &v20220315privatepreview.DaprInvokeHTTPRouteResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "new-resource-etag", w.Header().Get("ETag"))
			}
		})
	}

	updateExistingResourceTestCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		inputFile          string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"update-resource-no-if-match", "If-Match", "", "", "resource-etag", http.StatusOK, false},
		{"update-resource-with-diff-appid", "If-Match", "", "20220315privatepreview_input_appid.json", "resource-etag", http.StatusBadRequest, true},
		{"update-resource-*-if-match", "If-Match", "*", "", "resource-etag", http.StatusOK, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "", "matching-etag", http.StatusOK, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "", "another-etag", http.StatusPreconditionFailed, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "", "another-etag", http.StatusPreconditionFailed, true},
	}

	for _, testcase := range updateExistingResourceTestCases {
		t.Run(testcase.desc, func(t *testing.T) {
			input, dataModel, expectedOutput := getTestModels20220315privatepreview()
			if testcase.inputFile != "" {
				input = &v20220315privatepreview.DaprInvokeHTTPRouteResource{}
				_ = json.Unmarshal(radiustesting.ReadFixture(testcase.inputFile), input)
			}
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, input)
			req.Header.Set(testcase.headerKey, testcase.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: testcase.resourceETag},
						Data:     dataModel,
					}, nil
				})

			if !testcase.shouldFail {
				deploymentOutput.RadiusResource = dataModel
				deploymentOutput.RadiusResource.(*datamodel.DaprInvokeHttpRoute).Properties.AppId = rendererOutput.ComputedValues[daprinvokehttproutes.AppIDKey].Value.(string)
				mDeploymentProcessor.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(rendererOutput, nil)
				mDeploymentProcessor.EXPECT().Deploy(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(deploymentOutput, nil)
				mDeploymentProcessor.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(nil)

				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = dataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				GetDeploymentProcessor: func() deployment.DeploymentProcessor {
					return mDeploymentProcessor
				},
			}

			ctl, err := NewCreateOrUpdateDaprInvokeHttpRoute(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

			if !testcase.shouldFail {
				actualOutput := &v20220315privatepreview.DaprInvokeHTTPRouteResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "updated-resource-etag", w.Header().Get("ETag"))
			}
		})
	}
}
