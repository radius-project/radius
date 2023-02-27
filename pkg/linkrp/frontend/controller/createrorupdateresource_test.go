// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	radiustesting "github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCreateOrUpdateLinkResource_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	ctx := context.Background()

	for _, resourceType := range LinkTypes {
		if resourceType == linkrp.MongoDatabasesResourceType || resourceType == linkrp.RedisCachesResourceType {
			//MongoDatabases uses an async controller that has separate test code.
			continue
		}

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
		}

		for _, testcase := range createNewResourceTestCases {
			t.Run(resourceType+"-"+testcase.desc, func(t *testing.T) {
				rendererOutput, deploymentOutput := getDeploymentProcessorOutputs(resourceType, false)
				w := httptest.NewRecorder()
				input, dataModel, expectedOutput, actualOutput, testHeaderfile := createDataForLinkType(resourceType, false)
				req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, input)
				req.Header.Set(testcase.headerKey, testcase.headerValue)
				ctx := radiustesting.ARMTestContextFromRequest(req)

				if !testcase.daprMissing {
					mStorageClient.
						EXPECT().
						Get(gomock.Any(), gomock.Any()).
						DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
							return nil, &store.ErrNotFound{}
						})
				}

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

				kubeClient := radiustesting.NewFakeKubeClient(crdScheme, &apiextv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apiextensions.k8s.io/v1",
						Kind:       "CustomResourceDefinition",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "components.dapr.io",
					},
				})
				if testcase.daprMissing {
					kubeClient = radiustesting.NewFakeKubeClient(crdScheme) // Will return 404 for missing CRD
				}

				opts := Options{
					Options: ctrl.Options{
						StorageClient: mStorageClient,
						KubeClient:    kubeClient,
					},
					DeployProcessor: mDeploymentProcessor,
				}

				ctl, err := createCreateOrUpdateController(resourceType, opts)
				require.NoError(t, err)
				resp, err := ctl.Run(ctx, w, req)
				require.NoError(t, err)
				_ = resp.Apply(ctx, w, req)
				require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

				if !testcase.shouldFail {
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
			useDiff            bool
			resourceETag       string
			daprMissing        bool
			expectedStatusCode int
			shouldFail         bool
		}{
			{"update-resource-no-if-match", "If-Match", "", false, "resource-etag", false, http.StatusOK, false},
			{"update-resource-with-diff-app", "If-Match", "", true, "resource-etag", false, http.StatusBadRequest, true},
			{"update-resource-*-if-match", "If-Match", "*", false, "resource-etag", false, http.StatusOK, false},
			{"update-resource-matching-if-match", "If-Match", "matching-etag", false, "matching-etag", false, http.StatusOK, false},
			{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", false, "another-etag", false, http.StatusPreconditionFailed, true},
			{"update-resource-*-if-none-match", "If-None-Match", "*", false, "another-etag", false, http.StatusPreconditionFailed, true},
		}

		for _, testcase := range updateExistingResourceTestCases {
			t.Run(resourceType+"-"+testcase.desc, func(t *testing.T) {
				rendererOutput, deploymentOutput := getDeploymentProcessorOutputs(resourceType, false)
				w := httptest.NewRecorder()
				input, dataModel, expectedOutput, actualOutput, testHeaderfile := createDataForLinkType(resourceType, testcase.useDiff)
				req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, input)
				req.Header.Set(testcase.headerKey, testcase.headerValue)
				ctx := radiustesting.ARMTestContextFromRequest(req)

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
					mDeploymentProcessor.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)

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

				kubeClient := radiustesting.NewFakeKubeClient(crdScheme, &apiextv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apiextensions.k8s.io/v1",
						Kind:       "CustomResourceDefinition",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "components.dapr.io",
					},
				})
				if testcase.daprMissing {
					kubeClient = radiustesting.NewFakeKubeClient(crdScheme) // Will return 404 for missing CRD
				}

				opts := Options{
					Options: ctrl.Options{
						StorageClient: mStorageClient,
						KubeClient:    kubeClient,
					},
					DeployProcessor: mDeploymentProcessor,
				}

				ctl, err := createCreateOrUpdateController(resourceType, opts)

				require.NoError(t, err)
				resp, err := ctl.Run(ctx, w, req)
				_ = resp.Apply(ctx, w, req)
				require.NoError(t, err)
				require.Equal(t, testcase.expectedStatusCode, w.Result().StatusCode)

				if !testcase.shouldFail {
					_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
					require.Equal(t, expectedOutput, actualOutput)

					require.Equal(t, "updated-resource-etag", w.Header().Get("ETag"))
				}
			})
		}
	}
}

func getDeploymentProcessorOutputs(resourceType string, buildComputedValueReferences bool) (rendererOutput renderers.RendererOutput, deploymentOutput rpv1.DeploymentOutput) {
	switch strings.ToLower(resourceType) {
	case strings.ToLower(linkrp.DaprInvokeHttpRoutesResourceType):
		rendererOutput = renderers.RendererOutput{
			ComputedValues: map[string]renderers.ComputedValueReference{
				"appId": {
					Value: "test-appId",
				},
			},
		}

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{},
		}
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		output := rpv1.OutputResource{
			LocalID: rpv1.LocalIDAzureServiceBusNamespace,
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
			linkrp.NamespaceNameKey: {
				LocalID:           rpv1.LocalIDAzureServiceBusNamespace,
				PropertyReference: handlers.ServiceBusNamespaceNameKey,
			},
			linkrp.PubSubNameKey: {
				LocalID:           rpv1.LocalIDAzureServiceBusNamespace,
				PropertyReference: handlers.ResourceName,
			},
			linkrp.TopicNameKey: {
				Value: "test-topic",
			},
			linkrp.ComponentNameKey: {
				Value: "test-app-test-pub-sub-topic",
			},
		}
		rendererOutput = renderers.RendererOutput{
			Resources:      []rpv1.OutputResource{output},
			SecretValues:   map[string]rpv1.SecretValueReference{},
			ComputedValues: values,
		}

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{
				{
					LocalID: rpv1.LocalIDAzureServiceBusNamespace,
					ResourceType: resourcemodel.ResourceType{
						Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
						Provider: resourcemodel.ProviderAzure,
					},
				},
			},
			ComputedValues: map[string]any{
				linkrp.TopicNameKey:     rendererOutput.ComputedValues[linkrp.TopicNameKey].Value,
				linkrp.ComponentNameKey: rendererOutput.ComputedValues[linkrp.ComponentNameKey].Value,
			},
		}
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		rendererOutput = renderers.RendererOutput{
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
					Value: "test-app-test-secret-store",
				},
			},
		}

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{
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
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		rendererOutput = renderers.RendererOutput{
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

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{
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
	case strings.ToLower(linkrp.ExtendersResourceType):
		rendererOutput = renderers.RendererOutput{
			SecretValues: map[string]rpv1.SecretValueReference{
				"secretname": {
					Value: "secretvalue",
				},
			},
			ComputedValues: map[string]renderers.ComputedValueReference{
				"foo": {
					Value: "bar",
				},
			},
		}

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{},
		}
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		rendererOutput = renderers.RendererOutput{
			SecretValues: map[string]rpv1.SecretValueReference{
				renderers.ConnectionStringValue: {
					Value: "testConnectionString",
				},
			},
			ComputedValues: map[string]renderers.ComputedValueReference{
				"queue": {
					Value: "testqueue",
				},
			},
		}

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{},
		}
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		rendererOutput = renderers.RendererOutput{
			Resources: []rpv1.OutputResource{
				{
					LocalID: rpv1.LocalIDAzureSqlServer,
					ResourceType: resourcemodel.ResourceType{
						Type:     resourcekinds.AzureSqlServer,
						Provider: resourcemodel.ProviderAzure,
					},
					Identity: resourcemodel.ResourceIdentity{},
				},
			},
			SecretValues: map[string]rpv1.SecretValueReference{},
			ComputedValues: map[string]renderers.ComputedValueReference{
				linkrp.DatabaseNameValue: {
					Value: "db",
				},
				linkrp.ServerNameValue: {
					LocalID:     rpv1.LocalIDAzureSqlServer,
					JSONPointer: "/properties/fullyQualifiedDomainName",
				},
			},
		}

		deploymentOutput = rpv1.DeploymentOutput{
			DeployedOutputResources: []rpv1.OutputResource{
				{
					LocalID: rpv1.LocalIDAzureSqlServer,
					ResourceType: resourcemodel.ResourceType{
						Type:     resourcekinds.AzureSqlServer,
						Provider: resourcemodel.ProviderAzure,
					},
				},
			},
		}
	}

	return rendererOutput, deploymentOutput
}

func createCreateOrUpdateController(resourceType string, opts Options) (controller ctrl.Controller, err error) {
	switch strings.ToLower(resourceType) {
	case strings.ToLower(linkrp.DaprInvokeHttpRoutesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
			RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
			ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			true,
		)
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
			RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
			ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			true,
		)
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprSecretStore]{
			RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
			ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			true,
		)
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprStateStore]{
			RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
			ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			true,
		)
	case strings.ToLower(linkrp.ExtendersResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.Extender]{
			RequestConverter:  converter.ExtenderDataModelFromVersioned,
			ResponseConverter: converter.ExtenderDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			false,
		)
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
			RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
			ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			false,
		)
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.SqlDatabase]{
			RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
			ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewCreateOrUpdateResource(
			opts,
			operation,
			false,
		)
	}

	return
}
