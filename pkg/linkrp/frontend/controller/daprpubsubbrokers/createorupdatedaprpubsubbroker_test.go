// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

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
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprpubsubbrokers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func getDeploymentProcessorOutputs() (renderers.RendererOutput, deployment.DeploymentOutput) {
	output := outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureServiceBusNamespace,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
			Provider: resourcemodel.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.ResourceName:            "test-pub-sub-topic",
			handlers.KubernetesNamespaceKey:  "test-app",
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",

			// Truncate the topic part of the ID to make an ID for the namespace
			handlers.ServiceBusNamespaceIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace",
			handlers.ServiceBusNamespaceNameKey: "test-namespace",
			handlers.ServiceBusTopicNameKey:     "test-topic",
		},
	}
	values := map[string]renderers.ComputedValueReference{
		daprpubsubbrokers.NamespaceNameKey: {
			LocalID:           outputresource.LocalIDAzureServiceBusNamespace,
			PropertyReference: handlers.ServiceBusNamespaceNameKey,
		},
		daprpubsubbrokers.PubSubNameKey: {
			LocalID:           outputresource.LocalIDAzureServiceBusNamespace,
			PropertyReference: handlers.ResourceName,
		},
		daprpubsubbrokers.TopicNameKey: {
			Value: "test-topic",
		},
		renderers.ComponentNameKey: {
			Value: "test-app-test-pub-sub-topic",
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{output},
		SecretValues:   map[string]rp.SecretValueReference{},
		ComputedValues: values,
	}

	deploymentOutput := deployment.DeploymentOutput{
		Resources: []outputresource.OutputResource{
			{
				LocalID: outputresource.LocalIDAzureServiceBusNamespace,
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
					Provider: resourcemodel.ProviderAzure,
				},
			},
		},
		ComputedValues: map[string]interface{}{
			daprpubsubbrokers.TopicNameKey: rendererOutput.ComputedValues[daprpubsubbrokers.TopicNameKey].Value,
			renderers.ComponentNameKey:     rendererOutput.ComputedValues[renderers.ComponentNameKey].Value,
		},
	}

	return rendererOutput, deploymentOutput
}

func TestCreateOrUpdateDaprPubSubBroker_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	rendererOutput, deploymentOutput := getDeploymentProcessorOutputs()
	mStorageClient := store.NewMockStorageClient(mctrl)
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

			ctl, err := NewCreateOrUpdateDaprPubSubBroker(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

			if !testcase.shouldFail {
				actualOutput := &v20220315privatepreview.DaprPubSubBrokerResource{}
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
		{"create-new-resource-with-diff-envid", "If-Match", "", "20220315privatepreview_input_diff_env.json", "", http.StatusBadRequest, true},
		{"update-resource-*-if-match", "If-Match", "*", "", "resource-etag", http.StatusOK, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "", "matching-etag", http.StatusOK, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "", "another-etag", http.StatusPreconditionFailed, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "", "another-etag", http.StatusPreconditionFailed, true},
	}

	for _, testcase := range updateExistingResourceTestCases {
		t.Run(testcase.desc, func(t *testing.T) {
			input, dataModel, expectedOutput := getTestModels20220315privatepreview()
			if testcase.inputFile != "" {
				input = &v20220315privatepreview.DaprPubSubBrokerResource{}
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

			ctl, err := NewCreateOrUpdateDaprPubSubBroker(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

			if !testcase.shouldFail {
				actualOutput := &v20220315privatepreview.DaprPubSubBrokerResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "updated-resource-etag", w.Header().Get("ETag"))
			}
		})
	}
}
