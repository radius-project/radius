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

package awsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	queueinmemory "github.com/radius-project/radius/pkg/components/queue/inmemory"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/backend/controller/awsresources"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	vpcType    = "AWS::EC2::VPC"
	bucketType = "AWS::S3::Bucket"
	dbType     = "AWS::Test::Database"

	providers = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/"
	vpcPath   = providers + "AWS.EC2/VPC/app-vpc"
	vpc2Path  = providers + "AWS.EC2/VPC/other-vpc"
	bktPath   = providers + "AWS.S3/Bucket/app-bucket"
	bkt2Path  = providers + "AWS.S3/Bucket/other-bucket"
	dbPath    = providers + "AWS.Test/Database/app-db"

	otherOwner = "deploy;/planes/radius/local/resourceGroups/other;;"
	password   = "p@ssw0rd-never-stored"
)

var harnessSchemas = map[string]string{
	vpcType:    `{"primaryIdentifier": ["/properties/VpcId"], "readOnlyProperties": ["/properties/VpcId"], "createOnlyProperties": ["/properties/CidrBlock"]}`,
	bucketType: `{"primaryIdentifier": ["/properties/BucketName"], "readOnlyProperties": ["/properties/Arn"], "createOnlyProperties": ["/properties/BucketName"]}`,
	dbType: `{
		"primaryIdentifier": ["/properties/Name"],
		"createOnlyProperties": ["/properties/Name"],
		"writeOnlyProperties": ["/properties/Password", "/properties/Auth/Token", "/properties/Users/*/Secret"]
	}`,
}

// harness runs the AWS resource controllers and the backend worker against an in-memory data store, queue, and Cloud
// Control.
type harness struct {
	t     *testing.T
	cc    *awsfake.CloudControl
	db    database.Client
	queue *queueinmemory.Client
	sm    statusmanager.StatusManager
}

func newHarness(t *testing.T) *harness {
	cc := awsfake.NewCloudControl()
	cc.Identifiers[bucketType] = "BucketName"
	cc.Identifiers[dbType] = "Name"
	cc.WriteOnly[dbType] = []string{"Password", "Auth/Token", "Users/*/Secret"}
	db := inmemory.NewClient()
	q := queueinmemory.New(queueinmemory.NewInMemQueue(time.Minute))
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	return &harness{t: t, cc: cc, db: db, queue: q, sm: statusmanager.New(db, q, "global")}
}

func (h *harness) clients() ucp_aws.Clients {
	return ucp_aws.Clients{CloudControl: h.cc, CloudFormation: &awsfake.CloudFormation{Schemas: harnessSchemas}}
}

func (h *harness) options() armrpc_controller.Options {
	return armrpc_controller.Options{DatabaseClient: h.db, StatusManager: h.sm}
}

type response struct {
	status int
	body   map[string]any
	raw    string
}

func (r response) message() string {
	e, _ := r.body["error"].(map[string]any)
	m, _ := e["message"].(string)
	return m
}

func (h *harness) do(factory controllerFactory, method string, path string, owner string, body any) response {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewBuffer(b)
	}
	req, err := http.NewRequest(method, path, reader)
	require.NoError(h.t, err)
	req.Host, req.URL.Host, req.URL.Scheme = testHost, testHost, testScheme
	if owner != "" {
		req.Header.Set(sdkclients.AWSResourceOwnerHeader, owner)
	}
	controller, err := factory(h.options(), h.clients())
	require.NoError(h.t, err)
	ctx := rpctest.NewARMRequestContext(req)
	resp, err := controller.Run(ctx, nil, req)
	if err != nil {
		// The server turns a controller error into a 500.
		return response{status: http.StatusInternalServerError, raw: err.Error(), body: map[string]any{"error": map[string]any{"message": err.Error()}}}
	}
	w := httptest.NewRecorder()
	require.NoError(h.t, resp.Apply(ctx, w, req))
	res := w.Result()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(h.t, err)
	out := response{status: res.StatusCode, raw: string(raw), body: map[string]any{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (h *harness) put(path string, properties map[string]any) response {
	return h.do(NewCreateOrUpdateAWSResource, http.MethodPut, path, testOwner, map[string]any{"properties": properties})
}

func (h *harness) putBody(path string, owner string, body map[string]any) response {
	return h.do(NewCreateOrUpdateAWSResource, http.MethodPut, path, owner, body)
}

func (h *harness) delete(path string, owner string) response {
	return h.do(NewDeleteAWSResource, http.MethodDelete, path, owner, nil)
}

func (h *harness) get(path string) response {
	return h.do(NewGetAWSResource, http.MethodGet, path, "", nil)
}

// finish runs every queued operation the way the async worker does, and returns the results.
func (h *harness) finish() []ctrl.Result {
	h.t.Helper()
	results := []ctrl.Result{}
	for {
		msg, err := h.queue.Dequeue(h.t.Context(), queue.QueueClientConfig{})
		if errors.Is(err, queue.ErrMessageNotFound) {
			return results
		}
		require.NoError(h.t, err)
		req := &ctrl.Request{}
		require.NoError(h.t, json.Unmarshal(msg.Data, req))
		armCtx, err := req.ARMRequestContext()
		require.NoError(h.t, err)

		h.setState(req.ResourceID, v1.ProvisioningStateUpdating)
		controller, err := awsresources.NewAwaitController(ctrl.Options{DatabaseClient: h.db}, h.clients(), armCtx.OperationType.Method)
		require.NoError(h.t, err)
		result, err := controller.Run(h.t.Context(), req)
		require.NoError(h.t, err)
		h.setState(req.ResourceID, result.ProvisioningState())
		id, err := resources.ParseResource(req.ResourceID)
		require.NoError(h.t, err)
		require.NoError(h.t, h.sm.Update(h.t.Context(), id, req.OperationID, result.ProvisioningState(), nil, result.Error))
		require.NoError(h.t, h.queue.FinishMessage(h.t.Context(), msg))
		results = append(results, result)
	}
}

// setState sets the record's provisioning state the way the async worker does.
func (h *harness) setState(id string, state v1.ProvisioningState) {
	obj, err := h.db.Get(h.t.Context(), id)
	if errors.Is(err, &database.ErrNotFound{}) {
		return
	}
	require.NoError(h.t, err)
	obj.Data.(map[string]any)["provisioningState"] = string(state)
	require.NoError(h.t, h.db.Save(h.t.Context(), obj, database.WithETag(obj.ETag)))
}

func (h *harness) record(path string) *datamodel.AWSResource {
	h.t.Helper()
	id, err := resources.ParseResource(path)
	require.NoError(h.t, err)
	rec, _, err := records.NewStore(h.db).Get(h.t.Context(), id)
	require.NoError(h.t, err)
	return rec
}

func (h *harness) saveRecord(path string, change func(rec *datamodel.AWSResource)) {
	h.t.Helper()
	id, err := resources.ParseResource(path)
	require.NoError(h.t, err)
	store := records.NewStore(h.db)
	rec, etag, err := store.Get(h.t.Context(), id)
	require.NoError(h.t, err)
	if rec == nil {
		rec = records.NewRecord(id, testOwner)
		rec.SetProvisioningState(v1.ProvisioningStateSucceeded)
	}
	change(rec)
	_, err = store.Save(h.t.Context(), rec, etag)
	require.NoError(h.t, err)
}

func operations(calls []awsfake.Call) []string {
	out := []string{}
	for _, c := range calls {
		out = append(out, c.Operation)
	}
	return out
}

func Test_Put_VPCByAlias_Idempotent(t *testing.T) {
	h := newHarness(t)

	resp := h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Equal(t, vpcPath, resp.body["id"])
	require.Equal(t, "app-vpc", resp.body["name"])
	require.Equal(t, "Provisioning", resp.body["properties"].(map[string]any)["provisioningState"])

	// A second PUT while the create is running gets 409.
	resp = h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
	require.Equal(t, http.StatusConflict, resp.status)
	require.Contains(t, resp.message(), "still running")

	results := h.finish()
	require.Len(t, results, 1)
	require.Nil(t, results[0].Error)
	rec := h.record(vpcPath)
	require.Equal(t, "id-1", rec.Properties.AWSIdentifier)
	require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	require.Empty(t, rec.Properties.ClientToken)
	require.Equal(t, map[string]any{"CidrBlock": "10.0.0.0/16"}, rec.Properties.LastApplied)
	h.cc.Calls()

	// Redeploying the same template finds the record and changes nothing.
	resp = h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
	require.Equal(t, 1, h.cc.Count(vpcType))

	// A change updates the same VPC.
	resp = h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16", "EnableDnsSupport": true})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	calls := h.cc.Calls()
	require.Equal(t, []string{"GetResource", "UpdateResource"}, operations(calls))
	require.Equal(t, "id-1", calls[1].Identifier)
	h.finish()
	require.Equal(t, 1, h.cc.Count(vpcType))
	require.Equal(t, true, h.cc.Resource(vpcType, "id-1")["EnableDnsSupport"])

	// GET by alias reads the record, then AWS.
	resp = h.get(vpcPath)
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, "10.0.0.0/16", resp.body["properties"].(map[string]any)["CidrBlock"])
}

func Test_Put_RemovedPropertyIsCleared(t *testing.T) {
	h := newHarness(t)
	h.cc.Put(bucketType, "assets", map[string]any{"BucketName": "assets", "Arn": "arn", "SetOutsideRadius": "x"})

	// Taking over a bucket by its identifier property: no record yet, so nothing is cleared.
	resp := h.put(bktPath, map[string]any{"BucketName": "assets", "VersioningConfiguration": map[string]any{"Status": "Enabled"}, "Tags": []any{"a"}})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	h.finish()
	require.Equal(t, "assets", h.record(bktPath).Properties.AWSIdentifier)
	h.cc.Calls()

	// Removing properties Radius set clears them. A property set outside Radius stays.
	resp = h.put(bktPath, map[string]any{"BucketName": "assets"})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	calls := h.cc.Calls()
	require.Equal(t, []string{"GetResource", "UpdateResource"}, operations(calls))
	require.Contains(t, calls[1].Document, `"op":"remove","path":"/VersioningConfiguration"`)
	require.Contains(t, calls[1].Document, `"op":"remove","path":"/Tags"`)
	require.NotContains(t, calls[1].Document, "SetOutsideRadius")
	h.finish()
	require.Equal(t, map[string]any{"BucketName": "assets", "Arn": "arn", "SetOutsideRadius": "x"}, h.cc.Resource(bucketType, "assets"))
}

func Test_Put_NoChangeSavesLastApplied(t *testing.T) {
	h := newHarness(t)
	h.saveRecord(bktPath, func(rec *datamodel.AWSResource) {
		rec.Properties.AWSIdentifier = "assets"
		rec.Properties.LastApplied = map[string]any{"BucketName": "assets"}
	})
	h.cc.Put(bucketType, "assets", map[string]any{"BucketName": "assets", "Versioning": "on"})

	// The template starts setting a property that already matches AWS: no AWS call, but Radius now owns it.
	resp := h.put(bktPath, map[string]any{"BucketName": "assets", "Versioning": "on"})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
	require.Equal(t, map[string]any{"BucketName": "assets", "Versioning": "on"}, h.record(bktPath).Properties.LastApplied)

	// So removing it later clears it.
	resp = h.put(bktPath, map[string]any{"BucketName": "assets"})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Contains(t, h.cc.Calls()[1].Document, `"op":"remove","path":"/Versioning"`)
}

func Test_Put_RemovedCreateOnlyProperty(t *testing.T) {
	h := newHarness(t)
	h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {
		rec.Properties.AWSIdentifier = "vpc-1"
		rec.Properties.LastApplied = map[string]any{"CidrBlock": "10.0.0.0/16"}
	})
	h.cc.Put(vpcType, "vpc-1", map[string]any{"VpcId": "vpc-1", "CidrBlock": "10.0.0.0/16"})

	resp := h.put(vpcPath, map[string]any{})
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.Contains(t, resp.message(), "CidrBlock")
	require.NotContains(t, resp.message(), "10.0.0.0/16")
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
}

func Test_Put_DeletedOutsideRadiusIsCreatedAgain(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	h.finish()
	h.cc.Remove(vpcType, "id-1")

	require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	h.finish()
	require.Equal(t, "id-2", h.record(vpcPath).Properties.AWSIdentifier)
	require.Equal(t, 1, h.cc.Count(vpcType))
}

func Test_Put_Name(t *testing.T) {
	t.Run("takes over an existing resource", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Put(vpcType, "vpc-console", map[string]any{"VpcId": "vpc-console", "CidrBlock": "10.0.0.0/16"})
		resp := h.putBody(vpcPath, testOwner, map[string]any{"name": "vpc-console", "properties": map[string]any{"CidrBlock": "10.0.0.0/16"}})
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Equal(t, "vpc-console", h.record(vpcPath).Properties.AWSIdentifier)

		// A different name for the same alias is rejected.
		resp = h.putBody(vpcPath, testOwner, map[string]any{"name": "vpc-other", "properties": map[string]any{"CidrBlock": "10.0.0.0/16"}})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "vpc-console")
	})

	t.Run("that AWS does not have is rejected", func(t *testing.T) {
		h := newHarness(t)
		resp := h.putBody(vpcPath, testOwner, map[string]any{"name": "vpc-typo", "properties": map[string]any{"CidrBlock": "10.0.0.0/16"}})
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Contains(t, resp.message(), "vpc-typo")
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("that matches the identifier property creates the resource", func(t *testing.T) {
		h := newHarness(t)
		resp := h.putBody(bktPath, testOwner, map[string]any{"name": "assets", "properties": map[string]any{"BucketName": "assets"}})
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		h.finish()
		require.Equal(t, 1, h.cc.Count(bucketType))
	})

	t.Run("that is not a string is rejected", func(t *testing.T) {
		h := newHarness(t)
		resp := h.putBody(vpcPath, testOwner, map[string]any{"name": 1, "properties": map[string]any{}})
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	})
}

func Test_Put_IdentifierHeldByAnotherAlias(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)

		// The first create is still running, and its identifier is already on the record.
		resp := h.put(bkt2Path, map[string]any{"BucketName": "assets"})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "app-bucket")

		h.finish()
		resp = h.put(bkt2Path, map[string]any{"BucketName": "assets"})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Equal(t, 1, h.cc.Count(bucketType))
	})

	t.Run("create while AWS does not have the resource yet", func(t *testing.T) {
		h := newHarness(t)
		// The first create has not reached AWS yet, but its record has the identifier.
		h.saveRecord(bktPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "assets"
			rec.Properties.ClientToken = "6f1d1b8b-6a29-4d47-8f0b-5ad7c1d3e111"
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		resp := h.put(bkt2Path, map[string]any{"BucketName": "assets"})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "app-bucket")
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
		require.Nil(t, h.record(bkt2Path))
	})

	t.Run("take over by name of a VPC another alias created", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.finish()
		resp := h.putBody(vpc2Path, testOwner, map[string]any{"name": "id-1", "properties": map[string]any{"CidrBlock": "10.0.0.0/16"}})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "app-vpc")
		require.Nil(t, h.record(vpc2Path))
	})
}

func Test_Owner(t *testing.T) {
	h := newHarness(t)

	resp := h.putBody(vpcPath, "", map[string]any{"properties": map[string]any{}})
	require.Equal(t, http.StatusBadRequest, resp.status)
	require.Contains(t, resp.message(), sdkclients.AWSResourceOwnerHeader)
	resp = h.delete(vpcPath, "")
	require.Equal(t, http.StatusBadRequest, resp.status)
	require.Empty(t, h.cc.Calls())

	require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	h.finish()
	h.cc.Calls()

	// Another owner, such as the same app deployed to another environment, cannot change or delete it.
	resp = h.putBody(vpcPath, otherOwner, map[string]any{"properties": map[string]any{"CidrBlock": "10.0.0.0/16"}})
	require.Equal(t, http.StatusConflict, resp.status)
	require.Contains(t, resp.message(), testOwner)
	resp = h.delete(vpcPath, otherOwner)
	require.Equal(t, http.StatusConflict, resp.status)
	require.Empty(t, h.cc.Calls())

	// Owners are compared ignoring case.
	resp = h.putBody(vpcPath, strings.ToUpper(testOwner), map[string]any{"properties": map[string]any{"CidrBlock": "10.0.0.0/16"}})
	require.Equal(t, http.StatusOK, resp.status, resp.raw)
}

func Test_Put_AliasWithSlash(t *testing.T) {
	h := newHarness(t)
	resp := h.put(providers+"AWS.EC2/VPC/a%2Fb", map[string]any{})
	require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
	require.Contains(t, resp.message(), "must not contain '/'")
}

func Test_Put_InvalidBody(t *testing.T) {
	h := newHarness(t)
	req, err := http.NewRequest(http.MethodPut, vpcPath, strings.NewReader("{"))
	require.NoError(t, err)
	req.Header.Set(sdkclients.AWSResourceOwnerHeader, testOwner)
	controller, err := NewCreateOrUpdateAWSResource(h.options(), h.clients())
	require.NoError(t, err)
	resp, err := controller.Run(rpctest.NewARMRequestContext(req), nil, req)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	require.NoError(t, resp.Apply(t.Context(), w, req))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func Test_WriteOnlyValuesAreNeverStored(t *testing.T) {
	h := newHarness(t)
	properties := map[string]any{
		"Name":     "db",
		"Password": password,
		"Auth":     map[string]any{"Token": password, "Mode": "token"},
		"Users":    []any{map[string]any{"Name": "u", "Secret": password}},
	}

	// The pending values saved before AWS is called, the operation, and the response.
	resp := h.put(dbPath, properties)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.NotContains(t, resp.raw, password)
	h.requireNotStored()
	pending := h.record(dbPath).Properties.PendingLastApplied
	require.Equal(t, map[string]any{"Name": "db", "Auth": map[string]any{"Mode": "token"}, "Users": []any{map[string]any{"Name": "u"}}}, pending)

	// The last-applied properties after AWS succeeds.
	h.finish()
	h.requireNotStored()
	require.Equal(t, pending, h.record(dbPath).Properties.LastApplied)

	// An update in progress.
	properties["Auth"] = map[string]any{"Token": password, "Mode": "other"}
	resp = h.put(dbPath, properties)
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.NotContains(t, resp.raw, password)
	h.requireNotStored()
	h.finish()
	h.requireNotStored()
}

// requireNotStored checks that no record, operation status, or queued message holds the password.
func (h *harness) requireNotStored() {
	h.t.Helper()
	for _, rt := range []string{"AWS.Test/Database"} {
		result, err := h.db.Query(h.t.Context(), database.Query{RootScope: "/planes/aws/aws/accounts/1234567/regions/us-west-2", ResourceType: rt})
		require.NoError(h.t, err)
		require.NotEmpty(h.t, result.Items)
		for _, item := range result.Items {
			raw, err := json.Marshal(item.Data)
			require.NoError(h.t, err)
			require.NotContains(h.t, string(raw), password)
		}
	}
	result, err := h.db.Query(h.t.Context(), database.Query{RootScope: "/planes/aws/aws", ScopeRecursive: true, ResourceType: "System.AWS/operationStatuses"})
	require.NoError(h.t, err)
	for _, item := range result.Items {
		raw, err := json.Marshal(item.Data)
		require.NoError(h.t, err)
		require.NotContains(h.t, string(raw), password)
	}
}

func Test_Update_FailurePaths(t *testing.T) {
	setup := func(t *testing.T) *harness {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets", "A": "1"}).status)
		h.finish()
		h.cc.Calls()
		return h
	}
	previous := map[string]any{"BucketName": "assets", "A": "1"}

	t.Run("AWS rejects the update call", func(t *testing.T) {
		h := setup(t)
		h.cc.Errors["UpdateResource"] = awsError(&smithy.GenericAPIError{Code: "ValidationException", Message: "bad", Fault: smithy.FaultClient})
		resp := h.put(bktPath, map[string]any{"BucketName": "assets", "A": "2"})
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		rec := h.record(bktPath)
		require.Equal(t, previous, rec.Properties.LastApplied)
		require.Nil(t, rec.Properties.PendingLastApplied)
	})

	t.Run("AWS reports the update failed", func(t *testing.T) {
		h := setup(t)
		h.cc.FailNext = cctypes.HandlerErrorCodeInvalidRequest
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets", "A": "2"}).status)
		// The pending values are saved before AWS finishes, and the previous ones stay.
		rec := h.record(bktPath)
		require.Equal(t, map[string]any{"BucketName": "assets", "A": "2"}, rec.Properties.PendingLastApplied)
		require.Equal(t, previous, rec.Properties.LastApplied)

		results := h.finish()
		require.Equal(t, v1.CodeInvalid, results[0].Error.Code)
		rec = h.record(bktPath)
		require.Equal(t, previous, rec.Properties.LastApplied)
		require.Nil(t, rec.Properties.PendingLastApplied)
		require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())

		// A retry works.
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets", "A": "2"}).status)
		h.finish()
		require.Equal(t, map[string]any{"BucketName": "assets", "A": "2"}, h.record(bktPath).Properties.LastApplied)
	})

	t.Run("UCP stops after AWS accepted the update and before queuing it", func(t *testing.T) {
		h := setup(t)
		h.sm = failingQueue{h.sm}
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets", "A": "2"}).status)
		rec := h.record(bktPath)
		require.NotEmpty(t, rec.Properties.RequestToken)
		require.Equal(t, previous, rec.Properties.LastApplied)
		h.sm = statusmanager.New(h.db, h.queue, "global")
		h.cc.Calls()

		// The retry polls the request AWS accepted instead of sending the update again.
		resp := h.put(bktPath, map[string]any{"BucketName": "assets", "A": "2"})
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		require.Empty(t, h.cc.Calls())
		h.finish()
		rec = h.record(bktPath)
		require.Equal(t, map[string]any{"BucketName": "assets", "A": "2"}, rec.Properties.LastApplied)
		require.Empty(t, rec.Properties.RequestToken)
	})

	t.Run("a stale update is resumed and a fresh one blocks", func(t *testing.T) {
		h := setup(t)
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets", "A": "2"}).status)
		// The queued message is lost, so nothing polls the update.
		_, err := h.queue.Dequeue(t.Context(), queue.QueueClientConfig{})
		require.NoError(t, err)

		require.Equal(t, http.StatusConflict, h.delete(bktPath, testOwner).status)

		defer func() { records.Now = time.Now }()
		records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
		// A DELETE resumes the update and asks the caller to retry.
		resp := h.delete(bktPath, testOwner)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "interrupted update")
		records.Now = time.Now
		h.finish()
		require.Equal(t, map[string]any{"BucketName": "assets", "A": "2"}, h.record(bktPath).Properties.LastApplied)
	})
}

// failingQueue is a status manager whose queue is down.
type failingQueue struct {
	statusmanager.StatusManager
}

func (f failingQueue) QueueAsyncOperation(ctx context.Context, sCtx *v1.ARMRequestContext, options statusmanager.QueueOperationOptions) error {
	return errors.New("queue is down")
}

func (h *harness) withFailingQueue() { h.sm = failingQueue{h.sm} }

func Test_Create_FailurePaths(t *testing.T) {
	t.Run("AWS rejects the create call", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Errors["CreateResource"] = awsError(&smithy.GenericAPIError{Code: "ValidationException", Message: "bad", Fault: smithy.FaultClient})
		resp := h.put(vpcPath, map[string]any{"CidrBlock": "bad"})
		require.Equal(t, http.StatusBadRequest, resp.status, resp.raw)
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("AWS reports the create failed", func(t *testing.T) {
		h := newHarness(t)
		h.cc.FailNext = cctypes.HandlerErrorCodeInternalFailure
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		results := h.finish()
		require.Equal(t, v1.CodeInternal, results[0].Error.Code)
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("the create call fails in a way AWS may have accepted it", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Errors["CreateResource"] = errors.New("connection reset")
		resp := h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
		require.Equal(t, http.StatusInternalServerError, resp.status)
		rec := h.record(vpcPath)
		token := rec.Properties.ClientToken
		require.NotEmpty(t, token)
		require.Equal(t, v1.ProvisioningStateAccepted, rec.ProvisioningState())

		// The retry sends the create again with the same client token.
		delete(h.cc.Errors, "CreateResource")
		h.cc.Calls()
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		calls := h.cc.Calls()
		require.Equal(t, []string{"CreateResource"}, operations(calls))
		require.Equal(t, token, calls[0].ClientToken)
		h.finish()
		require.Equal(t, 1, h.cc.Count(vpcType))
	})

	t.Run("UCP stops after AWS accepted the create and before saving its request", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {
			rec.Properties.ClientToken = "6f1d1b8b-6a29-4d47-8f0b-5ad7c1d3e111"
			rec.Properties.PendingOperation = records.OperationCreate
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		// AWS accepted the first attempt.
		_, err := h.cc.CreateResource(t.Context(), &cloudcontrol.CreateResourceInput{
			TypeName: aws.String(vpcType), DesiredState: aws.String(`{"CidrBlock":"10.0.0.0/16"}`), ClientToken: aws.String("6f1d1b8b-6a29-4d47-8f0b-5ad7c1d3e111"),
		})
		require.NoError(t, err)
		h.cc.Calls()

		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.finish()
		require.Equal(t, 1, h.cc.Count(vpcType))
		rec := h.record(vpcPath)
		require.Equal(t, "id-1", rec.Properties.AWSIdentifier)
		require.Empty(t, rec.Properties.ClientToken)
	})

	t.Run("UCP stops after AWS accepted the create and before queuing it", func(t *testing.T) {
		h := newHarness(t)
		h.withFailingQueue()
		require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.sm = statusmanager.New(h.db, h.queue, "global")
		h.cc.Calls()

		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		require.Empty(t, h.cc.Calls())
		h.finish()
		require.Equal(t, 1, h.cc.Count(vpcType))
		require.Equal(t, "id-1", h.record(vpcPath).Properties.AWSIdentifier)
	})

	t.Run("a stale operation with no request is cleared", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(bktPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "assets"
			rec.Properties.LastApplied = map[string]any{"BucketName": "assets"}
			rec.Properties.PendingLastApplied = map[string]any{"BucketName": "assets", "A": "1"}
			rec.Properties.PendingOperation = records.OperationUpdate
			rec.SetProvisioningState(v1.ProvisioningStateUpdating)
		})
		h.cc.Put(bucketType, "assets", map[string]any{"BucketName": "assets"})
		defer func() { records.Now = time.Now }()
		records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
		resp := h.put(bktPath, map[string]any{"BucketName": "assets"})
		require.Equal(t, http.StatusOK, resp.status, resp.raw)
		require.Nil(t, h.record(bktPath).Properties.PendingLastApplied)
	})
}

func Test_Delete(t *testing.T) {
	t.Run("no record", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusNoContent, h.delete(vpcPath, testOwner).status)
		require.Empty(t, h.cc.Calls())
	})

	t.Run("record without an identifier", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {})
		require.Equal(t, http.StatusNoContent, h.delete(vpcPath, testOwner).status)
		require.Empty(t, h.cc.Calls())
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("resource AWS no longer has", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "vpc-gone" })
		require.Equal(t, http.StatusNoContent, h.delete(vpcPath, testOwner).status)
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("deletes by the record's identifier", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.finish()
		h.cc.Calls()

		resp := h.delete(vpcPath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		calls := h.cc.Calls()
		require.Equal(t, []string{"DeleteResource"}, operations(calls))
		require.Equal(t, "id-1", calls[0].Identifier)
		require.Equal(t, v1.ProvisioningStateDeleting, h.record(vpcPath).ProvisioningState())

		h.finish()
		require.Nil(t, h.record(vpcPath))
		require.Zero(t, h.cc.Count(vpcType))
	})

	t.Run("AWS reports the delete failed", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.finish()
		h.cc.FailNext = cctypes.HandlerErrorCodeResourceConflict
		require.Equal(t, http.StatusAccepted, h.delete(vpcPath, testOwner).status)
		results := h.finish()
		require.Equal(t, v1.CodeInvalid, results[0].Error.Code)
		rec := h.record(vpcPath)
		require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
		require.Equal(t, "id-1", rec.Properties.AWSIdentifier)
	})

	t.Run("AWS error", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "vpc-1" })
		h.cc.Errors["DeleteResource"] = awsError(&smithy.GenericAPIError{Code: "AccessDenied", Message: "no", Fault: smithy.FaultClient})
		require.Equal(t, http.StatusBadRequest, h.delete(vpcPath, testOwner).status)
		require.NotNil(t, h.record(vpcPath))
	})
}

func Test_Get(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusNotFound, h.get(vpcPath).status)
	require.Empty(t, h.cc.Calls())

	h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {})
	require.Equal(t, http.StatusNotFound, h.get(vpcPath).status)

	h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "vpc-1" })
	require.Equal(t, http.StatusNotFound, h.get(vpcPath).status)

	h.cc.Errors["GetResource"] = awsError(&smithy.GenericAPIError{Code: "Throttling", Message: "slow", Fault: smithy.FaultServer})
	require.Equal(t, http.StatusInternalServerError, h.get(vpcPath).status)
}

func Test_ReadAndSaveErrors(t *testing.T) {
	t.Run("reading the record fails", func(t *testing.T) {
		h := newHarness(t)
		h.db = brokenDB{Client: h.db}
		for _, run := range []func() response{
			func() response { return h.put(vpcPath, map[string]any{}) },
			func() response { return h.delete(vpcPath, testOwner) },
			func() response { return h.get(vpcPath) },
		} {
			require.Equal(t, http.StatusInternalServerError, run().status)
		}
		require.Empty(t, h.cc.Calls())
	})

	t.Run("searching the records fails", func(t *testing.T) {
		h := newHarness(t)
		h.db = brokenDB{Client: h.db, query: true}
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
	})

	t.Run("reading AWS fails", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Errors["GetResource"] = awsError(&smithy.GenericAPIError{Code: "Throttling", Message: "slow", Fault: smithy.FaultServer})
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
		require.Nil(t, h.record(bktPath))
	})
}

// brokenDB fails every read, or every query when query is set.
type brokenDB struct {
	database.Client
	query bool
}

func (b brokenDB) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	if b.query {
		return b.Client.Get(ctx, id, options...)
	}
	return nil, errors.New("database is down")
}

func (b brokenDB) Query(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	return nil, errors.New("database is down")
}

func Test_MustResume(t *testing.T) {
	rec := &datamodel.AWSResource{}
	require.False(t, mustResume(rec))
	rec.Properties.RequestToken = "t"
	require.True(t, mustResume(rec))
	rec.SetProvisioningState(v1.ProvisioningStateUpdating)
	rec.SystemData.LastModifiedAt = time.Now().UTC().Format(time.RFC3339Nano)
	require.False(t, mustResume(rec))
}

// awsError wraps an API error the way the AWS SDK returns it.
func awsError(err *smithy.GenericAPIError) error {
	return &smithy.OperationError{ServiceID: "CloudControl", OperationName: "Op", Err: &smithyhttp.ResponseError{Err: err}}
}

// saveFailingDB fails saves of AWS resource records after the first allow saves.
type saveFailingDB struct {
	database.Client
	allow *int
	err   error
}

func (s saveFailingDB) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if strings.Contains(strings.ToLower(obj.ID), "/providers/aws.") && !strings.Contains(strings.ToLower(obj.ID), "operationstatuses") {
		if *s.allow <= 0 {
			return s.err
		}
		*s.allow--
	}
	return s.Client.Save(ctx, obj, options...)
}

func (s saveFailingDB) Delete(ctx context.Context, id string, options ...database.DeleteOptions) error {
	if *s.allow <= 0 {
		return s.err
	}
	return s.Client.Delete(ctx, id, options...)
}

func (h *harness) failSavesAfter(n int, err error) {
	h.db = saveFailingDB{Client: h.db, allow: &n, err: err}
}

func Test_SaveFailures(t *testing.T) {
	down := errors.New("database is down")

	t.Run("saving a new record before the create", func(t *testing.T) {
		h := newHarness(t)
		h.failSavesAfter(0, down)
		require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		require.Empty(t, operations(h.cc.Calls()))
	})

	t.Run("another first deploy of the alias saves first", func(t *testing.T) {
		h := newHarness(t)
		h.failSavesAfter(0, &database.ErrConcurrency{})
		resp := h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "at the same time")
		require.Empty(t, operations(h.cc.Calls()))
	})

	t.Run("saving the request AWS accepted", func(t *testing.T) {
		h := newHarness(t)
		h.failSavesAfter(1, down)
		resp := h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
		require.Equal(t, http.StatusInternalServerError, resp.status)
		require.Contains(t, resp.raw, "Deploy again to continue it")
	})

	t.Run("marking the operation in progress", func(t *testing.T) {
		h := newHarness(t)
		h.failSavesAfter(2, down)
		// The worker marks the record in progress itself, so the PUT still succeeds.
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	})

	setupAfter := func(t *testing.T, saves int) *harness {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
		h.finish()
		h.failSavesAfter(saves, down)
		return h
	}
	setup := func(t *testing.T) *harness { return setupAfter(t, 0) }

	t.Run("saving the last-applied properties when nothing changed", func(t *testing.T) {
		h := setup(t)
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
	})

	t.Run("saving pending values before an update", func(t *testing.T) {
		h := setup(t)
		h.cc.Calls()
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets", "A": "1"}).status)
		require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
	})

	t.Run("removing the record of a resource AWS no longer has", func(t *testing.T) {
		// The reservation saves; removing the record fails.
		h := setupAfter(t, 1)
		h.cc.Remove(bucketType, "assets")
		resp := h.delete(bktPath, testOwner)
		require.Equal(t, http.StatusInternalServerError, resp.status)
		require.Contains(t, resp.raw, "Delete again")
	})

	t.Run("reserving the delete", func(t *testing.T) {
		h := setup(t)
		h.cc.Calls()
		require.Equal(t, http.StatusInternalServerError, h.delete(bktPath, testOwner).status)
		require.Empty(t, h.cc.Calls(), "AWS is not called before the reservation is saved")
	})

	t.Run("saving the delete AWS accepted", func(t *testing.T) {
		h := setupAfter(t, 1)
		require.Equal(t, http.StatusInternalServerError, h.delete(bktPath, testOwner).status)
	})

	t.Run("clearing a stale operation", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(bktPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "assets"
			rec.SetProvisioningState(v1.ProvisioningStateUpdating)
		})
		defer func() { records.Now = time.Now }()
		records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
		h.failSavesAfter(0, down)
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
	})

	t.Run("resuming an operation", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(bktPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "assets"
			rec.Properties.RequestToken = "6f1d1b8b-6a29-4d47-8f0b-5ad7c1d3e111"
			rec.SetProvisioningState(v1.ProvisioningStateAccepted)
		})
		h.failSavesAfter(0, down)
		require.Equal(t, http.StatusInternalServerError, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
	})
}

func Test_ResumeDelete(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	h.finish()
	require.Equal(t, http.StatusAccepted, h.delete(vpcPath, testOwner).status)
	// The queued message is lost, so nothing polls the delete.
	_, err := h.queue.Dequeue(t.Context(), queue.QueueClientConfig{})
	require.NoError(t, err)

	defer func() { records.Now = time.Now }()
	records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
	h.cc.Calls()
	resp := h.delete(vpcPath, testOwner)
	require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
	require.Empty(t, h.cc.Calls(), "the delete AWS accepted is polled, not sent again")
	records.Now = time.Now
	h.finish()
	require.Nil(t, h.record(vpcPath))
}

func Test_Put_NoStatusManager(t *testing.T) {
	h := newHarness(t)
	h.sm = nil
	require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	// The record keeps the request AWS accepted, so a later deploy polls it.
	require.NotEmpty(t, h.record(vpcPath).Properties.RequestToken)
}

func Test_Put_RequestTokenNotUUID(t *testing.T) {
	options := setupTest(t)
	options.AWSCloudFormationClient.EXPECT().DescribeType(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudformation.DescribeTypeOutput{Schema: aws.String(harnessSchemas[vpcType])}, nil)
	options.AWSCloudControlClient.EXPECT().CreateResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&cloudcontrol.CreateResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String("not-a-uuid")}}, nil)

	request, err := http.NewRequest(http.MethodPut, vpcPath, strings.NewReader(`{"properties": {"CidrBlock": "10.0.0.0/16"}}`))
	require.NoError(t, err)
	withOwner(request)
	controller, err := NewCreateOrUpdateAWSResource(options.ControllerOptions(), options.Clients())
	require.NoError(t, err)
	_, err = controller.Run(rpctest.NewARMRequestContext(request), nil, request)
	require.ErrorContains(t, err, "not a UUID")
}

func Test_Put_ReadingKnownResourceFails(t *testing.T) {
	h := newHarness(t)
	h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) { rec.Properties.AWSIdentifier = "vpc-1" })
	h.cc.Errors["GetResource"] = awsError(&smithy.GenericAPIError{Code: "Throttling", Message: "slow", Fault: smithy.FaultServer})
	require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
	require.Equal(t, []string{"GetResource"}, operations(h.cc.Calls()))
}

func Test_Put_SchemaWithoutPrimaryIdentifier(t *testing.T) {
	h := newHarness(t)
	harnessSchemas["AWS::Test::NoIdentifier"] = `{}`
	defer delete(harnessSchemas, "AWS::Test::NoIdentifier")
	resp := h.put(providers+"AWS.Test/NoIdentifier/x", map[string]any{})
	require.Equal(t, http.StatusInternalServerError, resp.status)
	require.Contains(t, resp.raw, "primaryIdentifier")
	require.Empty(t, h.cc.Calls())
}

func Test_Put_BodyWithoutProperties(t *testing.T) {
	h := newHarness(t)
	resp := h.putBody(vpcPath, testOwner, map[string]any{})
	require.Equal(t, http.StatusCreated, resp.status, resp.raw)
	require.Equal(t, `{}`, h.cc.Calls()[0].Document)
}

// Test_Reservation_RejectsCompetingRequests interleaves a second request while UCP waits on AWS for the first one. The
// record is reserved before every AWS call, so the second request gets 409 and the first one's values are kept.
func Test_Reservation_RejectsCompetingRequests(t *testing.T) {
	t.Run("a no-op PUT racing an update", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
		h.finish()

		var competing response
		h.cc.Before["UpdateResource"] = func() {
			competing = h.put(bktPath, map[string]any{"BucketName": "assets"})
		}
		resp := h.put(bktPath, map[string]any{"BucketName": "assets", "Tags": []any{"a"}})
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		require.Equal(t, http.StatusConflict, competing.status, competing.raw)
		require.Contains(t, competing.message(), "still running")

		h.finish()
		require.Equal(t, map[string]any{"BucketName": "assets", "Tags": []any{"a"}}, h.record(bktPath).Properties.LastApplied)

		// A later deploy that omits the tags clears them in AWS.
		require.Equal(t, http.StatusCreated, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)
		h.finish()
		require.Equal(t, map[string]any{"BucketName": "assets"}, h.cc.Resource(bucketType, "assets"))
	})

	t.Run("a PUT racing a delete", func(t *testing.T) {
		h := newHarness(t)
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.finish()

		var competing response
		h.cc.Before["DeleteResource"] = func() {
			competing = h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16", "EnableDnsSupport": true})
		}
		require.Equal(t, http.StatusAccepted, h.delete(vpcPath, testOwner).status)
		require.Equal(t, http.StatusConflict, competing.status, competing.raw)

		h.finish()
		require.Nil(t, h.record(vpcPath))
		require.Zero(t, h.cc.Count(vpcType))
	})

	t.Run("a DELETE racing a create", func(t *testing.T) {
		h := newHarness(t)
		var competing response
		h.cc.Before["CreateResource"] = func() {
			competing = h.delete(vpcPath, testOwner)
		}
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		require.Equal(t, http.StatusConflict, competing.status, competing.raw)
		h.finish()
		require.Equal(t, "id-1", h.record(vpcPath).Properties.AWSIdentifier)
	})
}

// Test_Reservation_Recovery covers a reservation that UCP saved but never followed with a tracked request, for example
// because UCP stopped while it called AWS. Once it is stale, the next request recovers it by operation.
func Test_Reservation_Recovery(t *testing.T) {
	stale := func(t *testing.T) {
		records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
		t.Cleanup(func() { records.Now = time.Now })
	}

	t.Run("update: the pending properties count as applied", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Put(bucketType, "assets", map[string]any{"BucketName": "assets", "Tags": []any{"a"}})
		h.saveRecord(bktPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "assets"
			rec.Properties.LastApplied = map[string]any{"BucketName": "assets"}
			rec.Properties.PendingLastApplied = map[string]any{"BucketName": "assets", "Tags": []any{"a"}}
			rec.Properties.PendingOperation = records.OperationUpdate
			rec.SetProvisioningState(v1.ProvisioningStateUpdating)
		})
		// While the reservation is fresh, a request waits.
		require.Equal(t, http.StatusConflict, h.put(bktPath, map[string]any{"BucketName": "assets"}).status)

		stale(t)
		h.cc.Calls()
		// The deploy omits the tags the interrupted update may have set, so they are cleared.
		resp := h.put(bktPath, map[string]any{"BucketName": "assets"})
		require.Equal(t, http.StatusCreated, resp.status, resp.raw)
		calls := h.cc.Calls()
		require.Contains(t, calls[len(calls)-1].Document, `"op":"remove","path":"/Tags"`)
		records.Now = time.Now
		h.finish()
		require.Equal(t, map[string]any{"BucketName": "assets"}, h.cc.Resource(bucketType, "assets"))
	})

	t.Run("delete: the delete is sent again", func(t *testing.T) {
		h := newHarness(t)
		h.cc.Put(vpcType, "vpc-1", map[string]any{"VpcId": "vpc-1"})
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "vpc-1"
			rec.Properties.PendingOperation = records.OperationDelete
			rec.SetProvisioningState(v1.ProvisioningStateDeleting)
		})
		stale(t)
		resp := h.delete(vpcPath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		records.Now = time.Now
		h.finish()
		require.Nil(t, h.record(vpcPath))
		require.Zero(t, h.cc.Count(vpcType))
	})

	t.Run("delete: a PUT resumes it and retries", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "vpc-gone"
			rec.Properties.PendingOperation = records.OperationDelete
			rec.SetProvisioningState(v1.ProvisioningStateDeleting)
		})
		stale(t)
		resp := h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"})
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "interrupted delete")
		require.Nil(t, h.record(vpcPath), "AWS no longer had it, so the record is removed")
	})

	t.Run("delete: AWS fails, then the error is returned", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {
			rec.Properties.AWSIdentifier = "vpc-1"
			rec.Properties.PendingOperation = records.OperationDelete
			rec.SetProvisioningState(v1.ProvisioningStateDeleting)
		})
		h.cc.Errors["DeleteResource"] = awsError(&smithy.GenericAPIError{Code: "Throttling", Message: "slow", Fault: smithy.FaultServer})
		stale(t)
		require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{}).status)
		rec := h.record(vpcPath)
		require.Empty(t, rec.Properties.PendingOperation)
		require.False(t, records.IsRunning(rec), "the reservation is released, so the next request does not wait")
	})

	t.Run("create: the create is sent again with the client token", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {
			rec.Properties.ClientToken = "6f1d1b8b-6a29-4d47-8f0b-5ad7c1d3e111"
			rec.Properties.PendingOperation = records.OperationCreate
			rec.SetProvisioningState(v1.ProvisioningStateProvisioning)
		})
		stale(t)
		require.Equal(t, http.StatusCreated, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		calls := h.cc.Calls()
		require.Equal(t, "6f1d1b8b-6a29-4d47-8f0b-5ad7c1d3e111", calls[len(calls)-1].ClientToken)
	})
}

// Test_Delete_UncertainCreate covers a DELETE of a record whose first create may have reached AWS: it has a client
// token but no identifier.
func Test_Delete_UncertainCreate(t *testing.T) {
	t.Run("create accepted, request token save fails, then DELETE", func(t *testing.T) {
		h := newHarness(t)
		// The reservation saves; saving the request AWS accepted fails.
		allow := 1
		inner := h.db
		h.db = saveFailingDB{Client: inner, allow: &allow, err: errors.New("database is down")}
		require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		h.db = inner
		require.Equal(t, 1, h.cc.Count(vpcType))
		rec := h.record(vpcPath)
		require.Empty(t, rec.Properties.AWSIdentifier)
		require.NotEmpty(t, rec.Properties.ClientToken)

		// The reservation is still fresh, so the DELETE does not remove the record.
		resp := h.delete(vpcPath, testOwner)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.NotNil(t, h.record(vpcPath))

		// Once it is stale, the DELETE recovers the create from AWS and deletes the VPC.
		records.Now = func() time.Time { return time.Now().Add(records.StaleAfter + time.Minute) }
		t.Cleanup(func() { records.Now = time.Now })
		h.cc.Calls()
		resp = h.delete(vpcPath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		calls := h.cc.Calls()
		require.Equal(t, []string{"CreateResource", "DeleteResource"}, operations(calls))
		require.Equal(t, rec.Properties.ClientToken, calls[0].ClientToken)
		require.Equal(t, "id-1", calls[1].Identifier)
		records.Now = time.Now
		h.finish()
		require.Nil(t, h.record(vpcPath))
		require.Zero(t, h.cc.Count(vpcType))
	})

	uncertain := func(t *testing.T) *harness {
		h := newHarness(t)
		h.cc.Errors["CreateResource"] = errors.New("connection reset")
		require.Equal(t, http.StatusInternalServerError, h.put(vpcPath, map[string]any{"CidrBlock": "10.0.0.0/16"}).status)
		delete(h.cc.Errors, "CreateResource")
		h.cc.Calls()
		return h
	}

	t.Run("AWS never had the create: it is created and then deleted", func(t *testing.T) {
		h := uncertain(t)
		require.Zero(t, h.cc.Count(vpcType))
		resp := h.delete(vpcPath, testOwner)
		require.Equal(t, http.StatusAccepted, resp.status, resp.raw)
		h.finish()
		require.Nil(t, h.record(vpcPath))
		require.Zero(t, h.cc.Count(vpcType))
	})

	t.Run("the create is still running: 409 and the create is tracked", func(t *testing.T) {
		h := uncertain(t)
		h.cc.PendingReads = 1
		resp := h.delete(vpcPath, testOwner)
		require.Equal(t, http.StatusConflict, resp.status, resp.raw)
		require.Contains(t, resp.message(), "still running in AWS")
		h.finish()
		rec := h.record(vpcPath)
		require.Equal(t, "id-1", rec.Properties.AWSIdentifier)
		require.Empty(t, rec.Properties.ClientToken)

		require.Equal(t, http.StatusAccepted, h.delete(vpcPath, testOwner).status)
		h.finish()
		require.Zero(t, h.cc.Count(vpcType))
	})

	t.Run("the create failed in AWS: only the record is removed", func(t *testing.T) {
		h := uncertain(t)
		h.cc.FailNext = cctypes.HandlerErrorCodeInvalidRequest
		require.Equal(t, http.StatusNoContent, h.delete(vpcPath, testOwner).status)
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("AWS rejects the create: only the record is removed", func(t *testing.T) {
		h := uncertain(t)
		h.cc.Errors["CreateResource"] = awsError(&smithy.GenericAPIError{Code: "ValidationException", Message: "bad", Fault: smithy.FaultClient})
		require.Equal(t, http.StatusNoContent, h.delete(vpcPath, testOwner).status)
		require.Nil(t, h.record(vpcPath))
	})

	t.Run("the create call fails: the record is kept", func(t *testing.T) {
		h := uncertain(t)
		h.cc.Errors["CreateResource"] = errors.New("connection reset")
		require.Equal(t, http.StatusInternalServerError, h.delete(vpcPath, testOwner).status)
		rec := h.record(vpcPath)
		require.NotEmpty(t, rec.Properties.ClientToken)
		require.False(t, records.IsRunning(rec))
	})

	t.Run("reading the create status fails: the record is kept", func(t *testing.T) {
		h := uncertain(t)
		h.cc.Errors["GetResourceRequestStatus"] = errors.New("throttled")
		require.Equal(t, http.StatusInternalServerError, h.delete(vpcPath, testOwner).status)
		rec := h.record(vpcPath)
		require.NotEmpty(t, rec.Properties.ClientToken)
		require.False(t, records.IsRunning(rec))
	})

	t.Run("a record with no client token has nothing in AWS", func(t *testing.T) {
		h := newHarness(t)
		h.saveRecord(vpcPath, func(rec *datamodel.AWSResource) {})
		require.Equal(t, http.StatusNoContent, h.delete(vpcPath, testOwner).status)
		require.Empty(t, h.cc.Calls())
	})
}
