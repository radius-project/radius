// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
	radiustesting "github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestDeleteRedisCache_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	ctx := context.Background()

	t.Parallel()
	for _, resourceType := range LinkTypes {
		if resourceType == linkrp.MongoDatabasesResourceType || resourceType == linkrp.RedisCachesResourceType {
			//MongoDatabases uses an async controller that has separate test code.
			continue
		}

		t.Run(resourceType+"-"+"delete non-existing resource", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, getTestHeaderFileName(resourceType), nil)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			opts := Options{
				Options: ctrl.Options{
					StorageClient: mStorageClient,
				},
				DeployProcessor: mDeploymentProcessor,
			}

			ctl, err := createDeleteController(resourceType, opts)

			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, http.StatusNoContent, result.StatusCode)

			body := result.Body
			defer body.Close()
			payload, err := io.ReadAll(body)
			require.NoError(t, err)
			require.Empty(t, payload, "response body should be empty")
		})

		existingResourceDeleteTestCases := []struct {
			desc               string
			ifMatchETag        string
			resourceETag       string
			expectedStatusCode int
			shouldFail         bool
		}{
			{"delete-existing-resource-no-if-match", "", "random-etag", http.StatusOK, false},
			{"delete-not-existing-resource-no-if-match", "", "", http.StatusNoContent, true},
			{"delete-existing-resource-matching-if-match", "matching-etag", "matching-etag", http.StatusOK, false},
			{"delete-existing-resource-not-matching-if-match", "not-matching-etag", "another-etag", http.StatusPreconditionFailed, true},
			{"delete-not-existing-resource-*-if-match", "*", "", http.StatusNoContent, true},
			{"delete-existing-resource-*-if-match", "*", "random-etag", http.StatusOK, false},
		}

		for _, testcase := range existingResourceDeleteTestCases {
			t.Run(resourceType+"-"+testcase.desc, func(t *testing.T) {
				w := httptest.NewRecorder()

				req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, getTestHeaderFileName(resourceType), nil)
				req.Header.Set("If-Match", testcase.ifMatchETag)

				ctx := radiustesting.ARMTestContextFromRequest(req)
				redisDataModel := new(datamodel.RedisCache)
				getTestDataModel20220315privatepreview(redisDataModel)

				mStorageClient.
					EXPECT().
					Get(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
						return &store.Object{
							Metadata: store.Metadata{ID: id, ETag: testcase.resourceETag},
							Data:     redisDataModel,
						}, nil
					})

				if !testcase.shouldFail {
					mDeploymentProcessor.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
					mStorageClient.
						EXPECT().
						Delete(gomock.Any(), gomock.Any()).
						DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
							return nil
						})
				}

				opts := Options{
					Options: ctrl.Options{
						StorageClient: mStorageClient,
					},
					DeployProcessor: mDeploymentProcessor,
				}

				ctl, err := createDeleteController(resourceType, opts)

				require.NoError(t, err)
				resp, err := ctl.Run(ctx, w, req)
				require.NoError(t, err)
				err = resp.Apply(ctx, w, req)
				require.NoError(t, err)

				result := w.Result()
				require.Equal(t, testcase.expectedStatusCode, result.StatusCode)

				body := result.Body
				defer body.Close()
				payload, err := io.ReadAll(body)
				require.NoError(t, err)

				if result.StatusCode == http.StatusOK || result.StatusCode == http.StatusNoContent {
					// We return either 200 or 204 without a response body for success.
					require.Empty(t, payload, "response body should be empty")
				} else {
					armerr := v1.ErrorResponse{}
					err = json.Unmarshal(payload, &armerr)
					require.NoError(t, err)
					require.Equal(t, v1.CodePreconditionFailed, armerr.Error.Code)
					require.NotEmpty(t, armerr.Error.Target)
				}
			})

			t.Run(resourceType+"-"+"delete deploymentprocessor error", func(t *testing.T) {
				req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, getTestHeaderFileName(resourceType), nil)
				ctx := radiustesting.ARMTestContextFromRequest(req)
				dataModel := createDataModelForLinkType(resourceType)
				w := httptest.NewRecorder()

				mStorageClient.
					EXPECT().
					Get(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
						return &store.Object{
							Metadata: store.Metadata{ID: id, ETag: "test-etag"},
							Data:     dataModel,
						}, nil
					})
				mDeploymentProcessor.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(errors.New("deploymentprocessor: failed to delete the output resource"))

				opts := Options{
					Options: ctrl.Options{
						StorageClient: mStorageClient,
					},
					DeployProcessor: mDeploymentProcessor,
				}

				ctl, err := createDeleteController(resourceType, opts)
				require.NoError(t, err)
				_, err = ctl.Run(ctx, w, req)
				require.Error(t, err)
			})
		}
	}
}

func createDeleteController(resourceType string, opts Options) (controller ctrl.Controller, err error) {
	switch strings.ToLower(resourceType) {
	case strings.ToLower(linkrp.DaprInvokeHttpRoutesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
			RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
			ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
			RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
			ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprSecretStore]{
			RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
			ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.DaprStateStore]{
			RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
			ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.ExtendersResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.Extender]{
			RequestConverter:  converter.ExtenderDataModelFromVersioned,
			ResponseConverter: converter.ExtenderDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.MongoDatabasesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.MongoDatabase]{
			RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
			ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
			RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
			ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		resourceOptions := ctrl.ResourceOptions[datamodel.SqlDatabase]{
			RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
			ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
		}

		operation := ctrl.NewOperation(opts.Options,
			resourceOptions)

		return NewDeleteResource(
			opts,
			operation,
		)
	}

	return
}
