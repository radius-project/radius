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

package appsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	queueinmemory "github.com/radius-project/radius/pkg/components/queue/inmemory"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/awsresources"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/awsproxy"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// Test_DeployAfterFailedSchema deploys a definition AppSync fails to create, through UCP's controllers and worker,
// then deploys a fixed one. The failure changed nothing in AWS, so the second deploy sends StartSchemaCreation again
// with the fixed definition, instead of returning the first failure.
func Test_DeployAfterFailedSchema(t *testing.T) {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	store := awsfake.NewCloudControl()
	api := newFakeAPI(store, apiID)
	catalog, err := schemas.LoadFS(fstest.MapFS{
		schemas.NativeDir + "/aws-appsync-graphqlschema.json":          {Data: []byte(read(t, "aws-appsync-graphqlschema.json"))},
		schemas.NativeDir + "/aws-appsync-graphqlschema.metadata.json": {Data: []byte(read(t, "aws-appsync-graphqlschema.metadata.json"))},
		schemas.HandWrittenDir + "/README.md":                          {},
	})
	require.NoError(t, err)
	registry, err := native.NewRegistry(catalog, NewGraphQLSchema(api, api))
	require.NoError(t, err)
	clients := ucp_aws.Clients{
		CloudControl: store,
		CloudFormation: &awsfake.CloudFormation{
			Schemas:           map[string]string{schemaType: read(t, "aws-appsync-graphqlschema.json")},
			ProvisioningTypes: map[string]cftypes.ProvisioningType{schemaType: cftypes.ProvisioningTypeNonProvisionable},
		},
		Native: registry,
	}
	db := inmemory.NewClient()
	q := queueinmemory.New(queueinmemory.NewInMemQueue(time.Minute))
	opts := armrpc_controller.Options{DatabaseClient: db, StatusManager: statusmanager.New(db, q, "global")}
	key, err := ucp_aws.NewFingerprintKey(bytes.Repeat([]byte{7}, ucp_aws.MinFingerprintKeyLength))
	require.NoError(t, err)
	path := "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.AppSync/GraphQLSchema/app-schema"

	put := func(definition string) int {
		b, err := json.Marshal(map[string]any{"properties": map[string]any{"ApiId": apiID, "Definition": definition}})
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(b))
		require.NoError(t, err)
		req.Host, req.URL.Host, req.URL.Scheme = "localhost:5000", "localhost:5000", "http"
		req.Header.Set(sdkclients.AWSResourceOwnerHeader, "deploy;/planes/radius/local/resourceGroups/rg;;")
		controller, err := awsproxy.NewCreateOrUpdateAWSResource(opts, clients, func() (*ucp_aws.FingerprintKey, error) { return key, nil })
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)
		resp, err := controller.Run(ctx, nil, req)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		require.NoError(t, resp.Apply(ctx, w, req))
		raw, err := io.ReadAll(w.Result().Body)
		require.NoError(t, err)
		require.Less(t, w.Code, http.StatusBadRequest, string(raw))
		return w.Code
	}
	// finish runs the queued operation as UCP's worker does, and returns its error, or nil when it succeeded.
	finish := func() *v1.ErrorDetails {
		msg, err := q.Dequeue(t.Context(), queue.QueueClientConfig{})
		require.NoError(t, err)
		req := &ctrl.Request{}
		require.NoError(t, json.Unmarshal(msg.Data, req))
		armCtx, err := req.ARMRequestContext()
		require.NoError(t, err)
		controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: db}, clients, armCtx.OperationType.Method)
		require.NoError(t, err)
		result, err := controller.Run(t.Context(), req)
		require.NoError(t, err)
		require.NoError(t, q.FinishMessage(t.Context(), msg))
		_, err = q.Dequeue(t.Context(), queue.QueueClientConfig{})
		require.True(t, errors.Is(err, queue.ErrMessageNotFound), "one operation was queued")
		return result.Error
	}

	require.Equal(t, http.StatusCreated, put("type Query { invalid"))
	failure := finish()
	require.NotNil(t, failure)
	require.Contains(t, failure.Message, "Syntax Error")

	api.calls = nil
	require.Equal(t, http.StatusCreated, put("type Query { hello: String }"))
	require.Contains(t, api.calls, "StartSchemaCreation "+region, "the fixed definition is sent")
	require.Equal(t, "type Query { hello: String }", api.store.Resource(schemaType, apiID)["Definition"])
	require.Nil(t, finish())

	id, err := resources.ParseResource(path)
	require.NoError(t, err)
	rec, _, err := records.NewStore(db).Get(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	require.False(t, records.IsFirstCreate(rec))
}
