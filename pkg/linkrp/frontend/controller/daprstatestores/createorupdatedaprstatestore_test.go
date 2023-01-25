// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCreateOrUpdateDaprStateStore_20220315PrivatePreview(t *testing.T) {
	ctx := context.Background()

	createNewResourceTestCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		daprMissing        bool
		expectedStatusCode int
		shouldFail         bool
	}{
		{"create-new-resource-no-if-match", "If-Match", "", "", false, http.StatusOK, false},
		{"create-new-resource-*-if-match", "If-Match", "*", "", false, http.StatusPreconditionFailed, true},
		{"create-new-resource-etag-if-match", "If-Match", "random-etag", "", false, http.StatusPreconditionFailed, true},
		{"create-new-resource-*-if-none-match", "If-None-Match", "*", "", false, http.StatusOK, false},
		{"create-new-resource-without-dapr-installed", "If-Match", "", "", true, http.StatusBadRequest, true},
	}

	for _, testcase := range createNewResourceTestCases {
		t.Run(testcase.desc, func(t *testing.T) {
			input, dataModel, expectedOutput := getTestModels20220315privatepreview()
			rendererOutput, deploymentOutput := getDeploymentProcessorOutputs()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, input)
			req.Header.Set(testcase.headerKey, testcase.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			mctrl := gomock.NewController(t)
			mStorageClient := store.NewMockStorageClient(mctrl)
			mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)

			if !testcase.daprMissing {
				mStorageClient.
					EXPECT().
					Get(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
						return nil, &store.ErrNotFound{}
					})
			}

			expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
			expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
			expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

			if !testcase.shouldFail {
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

			// Most tests will cover the case where Dapr is installed.
			crdScheme := runtime.NewScheme()
			err := apiextv1.AddToScheme(crdScheme)
			require.NoError(t, err)

			kubeClient := testutil.NewFakeKubeClient(crdScheme, &apiextv1.CustomResourceDefinition{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apiextensions.k8s.io/v1",
					Kind:       "CustomResourceDefinition",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "components.dapr.io",
				},
			})
			if testcase.daprMissing {
				kubeClient = testutil.NewFakeKubeClient(crdScheme) // Will return 404 for missing CRD
			}

			opts := frontend_ctrl.Options{
				Options: ctrl.Options{
					StorageClient: mStorageClient,
					KubeClient:    kubeClient,
				},
				DeployProcessor: mDeploymentProcessor,
			}

			ctl, err := NewCreateOrUpdateDaprStateStore(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

			if !testcase.shouldFail {
				actualOutput := &v20220315privatepreview.DaprStateStoreResource{}
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
		daprMissing        bool
		expectedStatusCode int
		shouldFail         bool
	}{
		{"update-resource-no-if-match", "If-Match", "", "", "resource-etag", false, http.StatusOK, false},
		{"update-resource-with-diff-app", "If-Match", "", "20220315privatepreview_input_diff_app.json", "resource-etag", false, http.StatusBadRequest, true},
		{"update-resource-without-dapr-installed", "If-Match", "", "", "resource-etag", true, http.StatusBadRequest, true},
		{"update-resource-*-if-match", "If-Match", "*", "", "resource-etag", false, http.StatusOK, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "", "matching-etag", false, http.StatusOK, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "", "another-etag", false, http.StatusPreconditionFailed, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "", "another-etag", false, http.StatusPreconditionFailed, true},
	}

	for _, testcase := range updateExistingResourceTestCases {
		t.Run(testcase.desc, func(t *testing.T) {
			input, dataModel, expectedOutput := getTestModels20220315privatepreview()
			if testcase.inputFile != "" {
				input = &v20220315privatepreview.DaprStateStoreResource{}
				_ = json.Unmarshal(testutil.ReadFixture(testcase.inputFile), input)
			}
			rendererOutput, deploymentOutput := getDeploymentProcessorOutputs()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, input)
			req.Header.Set(testcase.headerKey, testcase.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			mctrl := gomock.NewController(t)
			mStorageClient := store.NewMockStorageClient(mctrl)
			mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)

			if !testcase.daprMissing {
				mStorageClient.
					EXPECT().
					Get(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
						return &store.Object{
							Metadata: store.Metadata{ID: id, ETag: testcase.resourceETag},
							Data:     dataModel,
						}, nil
					})
			}

			if !testcase.shouldFail {
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

			// Most tests will cover the case where Dapr is installed.
			crdScheme := runtime.NewScheme()
			err := apiextv1.AddToScheme(crdScheme)
			require.NoError(t, err)

			kubeClient := testutil.NewFakeKubeClient(crdScheme, &apiextv1.CustomResourceDefinition{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apiextensions.k8s.io/v1",
					Kind:       "CustomResourceDefinition",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "components.dapr.io",
				},
			})
			if testcase.daprMissing {
				kubeClient = testutil.NewFakeKubeClient(crdScheme) // Will return 404 for missing CRD
			}

			opts := frontend_ctrl.Options{
				Options: ctrl.Options{
					StorageClient: mStorageClient,
					KubeClient:    kubeClient,
				},
				DeployProcessor: mDeploymentProcessor,
			}

			ctl, err := NewCreateOrUpdateDaprStateStore(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

			if !testcase.shouldFail {
				actualOutput := &v20220315privatepreview.DaprStateStoreResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "updated-resource-etag", w.Header().Get("ETag"))
			}
		})
	}
}

func getDeploymentProcessorOutputs() (renderers.RendererOutput, deployment.DeploymentOutput) {
	rendererOutput := renderers.RendererOutput{
		Resources: []rpv1.OutputResource{
			{
				LocalID: rpv1.LocalIDDaprComponent,
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.DaprComponent,
					Provider: resourcemodel.ProviderKubernetes,
				},
				Identity: resourcemodel.ResourceIdentity{},
			},
		},
		SecretValues: map[string]rpv1.SecretValueReference{},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"componentName": {
				Value: "test-app-test-resource",
			},
		},
	}

	deploymentOutput := deployment.DeploymentOutput{
		Resources: []rpv1.OutputResource{
			{
				LocalID: rpv1.LocalIDDaprComponent,
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.DaprComponent,
					Provider: resourcemodel.ProviderKubernetes,
				},
			},
		},
		ComputedValues: map[string]any{
			"componentName": rendererOutput.ComputedValues["componentName"].Value,
		},
	}

	return rendererOutput, deploymentOutput
}
