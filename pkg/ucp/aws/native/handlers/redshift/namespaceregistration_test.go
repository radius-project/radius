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

package redshift

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const registrationTestRegion = "us-west-2"

// registrationSchema returns the hand-written schema of AWS::Redshift::NamespaceRegistration.
func registrationSchema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/aws-redshift-namespaceregistration.json")
	require.NoError(t, err)
	return string(b)
}

// serverlessRegistration and provisionedRegistration return the properties of a registration of a serverless
// namespace and workgroup, and of a cluster, with a consumer.
func serverlessRegistration(namespace string, workgroup string, consumer any) map[string]any {
	return map[string]any{
		"NamespaceIdentifier": map[string]any{"ServerlessIdentifier": map[string]any{"NamespaceIdentifier": namespace, "WorkgroupIdentifier": workgroup}},
		"ConsumerIdentifiers": []any{consumer},
	}
}

func provisionedRegistration(cluster string, consumer any) map[string]any {
	return map[string]any{
		"NamespaceIdentifier": map[string]any{"ProvisionedIdentifier": map[string]any{"ClusterIdentifier": cluster}},
		"ConsumerIdentifiers": []any{consumer},
	}
}

func newRegistrationAPI() *registrationFake {
	store := awsfake.NewCloudControl()
	addRegistrationParents(store)
	return newRegistrationFake(store)
}

func Test_Suite_NamespaceRegistration(t *testing.T) {
	invalid := []map[string]any{
		{"NamespaceIdentifier": analyticsProps()["NamespaceIdentifier"], "ConsumerIdentifiers": []any{registrationConsumer, registrationOtherConsumer}},
		{"NamespaceIdentifier": analyticsProps()["NamespaceIdentifier"], "ConsumerIdentifiers": []any{}},
		{"NamespaceIdentifier": analyticsProps()["NamespaceIdentifier"], "ConsumerIdentifiers": registrationConsumer},
		serverlessRegistration("analytics", "analytics-wg", "consumer"),
		serverlessRegistration("analytics", "analytics-wg", 444455556666),
		serverlessRegistration("Analytics", "analytics-wg", registrationConsumer),
		serverlessRegistration("analytics", "wg", registrationConsumer),
		provisionedRegistration("Warehouse", registrationConsumer),
		{"NamespaceIdentifier": map[string]any{}, "ConsumerIdentifiers": []any{registrationConsumer}},
		{"NamespaceIdentifier": "analytics", "ConsumerIdentifiers": []any{registrationConsumer}},
		{"NamespaceIdentifier": map[string]any{
			"ServerlessIdentifier":  map[string]any{"NamespaceIdentifier": "analytics", "WorkgroupIdentifier": "analytics-wg"},
			"ProvisionedIdentifier": map[string]any{"ClusterIdentifier": "warehouse"},
		}, "ConsumerIdentifiers": []any{registrationConsumer}},
		{"NamespaceIdentifier": map[string]any{"ServerlessIdentifier": map[string]any{"NamespaceIdentifier": "analytics"}}, "ConsumerIdentifiers": []any{registrationConsumer}},
		{"NamespaceIdentifier": map[string]any{"ProvisionedIdentifier": map[string]any{"ClusterIdentifier": "warehouse", "Region": "us-east-1"}}, "ConsumerIdentifiers": []any{registrationConsumer}},
		{"NamespaceIdentifier": map[string]any{"ServerlessIdentifier": map[string]any{"NamespaceIdentifier": 5, "WorkgroupIdentifier": "analytics-wg"}}, "ConsumerIdentifiers": []any{registrationConsumer}},
		{"NamespaceIdentifier": map[string]any{"Workgroup": map[string]any{"WorkgroupIdentifier": "analytics-wg"}}, "ConsumerIdentifiers": []any{registrationConsumer}},
	}
	for _, tt := range []struct {
		name       string
		properties map[string]any
		identifier string
		other      map[string]any
	}{
		{"serverless", analyticsProps(), "analytics|analytics-wg|" + registrationConsumer, serverlessRegistration("analytics", "analytics-wg", registrationOtherConsumer)},
		{"provisioned", provisionedRegistration("warehouse", registrationConsumer), "warehouse|" + registrationConsumer, provisionedRegistration("warehouse", registrationOtherConsumer)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nativetest.Run(t, nativetest.Suite{
				TypeName:    registrationType,
				Schema:      registrationSchema(t),
				HandWritten: true,
				NewHandler: func(store *awsfake.CloudControl) native.Handler {
					addRegistrationParents(store)
					api := newRegistrationFake(store)
					return NewNamespaceRegistration(api, api)
				},
				Properties:       tt.properties,
				Identifier:       tt.identifier,
				Changed:          tt.properties,
				CreateOnlyChange: tt.other,
				IdentifierProperties: func(identifier string) map[string]any {
					r, err := splitRegistration(identifier)
					require.NoError(t, err)
					return r.properties()
				},
				Invalid: invalid,
				API:     registrationAPI,
			})
		})
	}
}

// analyticsProps returns the properties of a registration of the fake's serverless namespace analytics.
func analyticsProps() map[string]any {
	return serverlessRegistration("analytics", "analytics-wg", registrationConsumer)
}

// registrationProgress calls Progress until op is done, and fails if it is done the first time, because a change
// leaves the cluster or serverless namespace busy. It returns the states Progress reported while it waited.
func registrationProgress(t *testing.T, h *NamespaceRegistration, identifier string, op native.Operation) []string {
	t.Helper()
	states := []string{}
	for i := range 10 {
		progress, err := h.Progress(t.Context(), registrationTestRegion, identifier, op)
		if op == native.OperationDelete && i > 0 && err != nil {
			require.ErrorIs(t, err, native.ErrNotFound)
			return states
		}
		require.NoError(t, err)
		if progress.Done {
			require.NotZero(t, i, "AWS is still changing the namespace when the code first looks")
			require.Nil(t, progress.Failure)
			return states
		}
		states = append(states, progress.State)
	}
	t.Fatal("the operation never finished")
	return nil
}

func Test_NamespaceRegistration_Lifecycle(t *testing.T) {
	for _, tt := range []struct {
		name       string
		properties map[string]any
		busy       string
	}{
		{"serverless", analyticsProps(), "workgroup MODIFYING"},
		{"provisioned", provisionedRegistration("warehouse", registrationConsumer), "cluster modifying"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := newRegistrationAPI()
			h := NewNamespaceRegistration(api, api)
			id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: tt.properties})
			require.NoError(t, err)
			_, err = h.Read(t.Context(), registrationTestRegion, id)
			require.ErrorIs(t, err, native.ErrNotFound, "AWS doesn't list the registration while the namespace is busy")
			require.Equal(t, []string{tt.busy}, registrationProgress(t, h, id, native.OperationCreate))
			properties, err := h.Read(t.Context(), registrationTestRegion, id)
			require.NoError(t, err)
			require.Equal(t, tt.properties, properties)

			require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: registrationTestRegion, Identifier: id, Desired: tt.properties}))
			progress, err := h.Progress(t.Context(), registrationTestRegion, id, native.OperationUpdate)
			require.NoError(t, err)
			require.True(t, progress.Done, "an update changes nothing")

			require.NoError(t, h.Delete(t.Context(), registrationTestRegion, id))
			require.Equal(t, []string{string(types.DataShareStatusAuthorized), tt.busy}, registrationProgress(t, h, id, native.OperationDelete),
				"a delete waits until AWS no longer lists the registration and the namespace is free")
			require.Equal(t, 1, api.calls["DeregisterNamespace"])
		})
	}
}

// Test_NamespaceRegistration_Busy checks that a registration AWS rejects because the namespace is busy fails with
// 409, naming dependsOn, and changes nothing.
func Test_NamespaceRegistration_Busy(t *testing.T) {
	api := newRegistrationAPI()
	h := NewNamespaceRegistration(api, api)
	api.held[registrationWorkgroupType+"/analytics-wg"] = true
	_, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "dependsOn")
	require.Contains(t, failure.Message, "analytics")
	require.Zero(t, api.store.Count(registrationType))

	delete(api.held, registrationWorkgroupType+"/analytics-wg")
	id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.NoError(t, err)
	err = h.Delete(t.Context(), registrationTestRegion, id)
	require.ErrorAs(t, err, &failure, "the namespace is still busy with the registration")
	require.True(t, failure.Conflict)

	api.held[registrationClusterType+"/warehouse"] = true
	err = h.Delete(t.Context(), registrationTestRegion, "warehouse|"+registrationConsumer)
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.Contains(t, failure.Message, "The cluster warehouse")
}

// Test_NamespaceRegistration_OtherNamespace checks that a serverless registration whose workgroup belongs to another
// namespace is never sent, because AWS would register the workgroup's namespace, and reads as absent.
func Test_NamespaceRegistration_OtherNamespace(t *testing.T) {
	api := newRegistrationAPI()
	h := NewNamespaceRegistration(api, api)
	for _, properties := range []map[string]any{
		serverlessRegistration("sales", "analytics-wg", registrationConsumer),
		serverlessRegistration("analytics", "missing-wg", registrationConsumer),
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: properties})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	}
	require.Zero(t, api.calls["RegisterNamespace"])

	id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.NoError(t, err)
	registrationProgress(t, h, id, native.OperationCreate)
	for _, other := range []string{"sales|analytics-wg|" + registrationConsumer, "analytics|missing-wg|" + registrationConsumer} {
		_, err = h.Read(t.Context(), registrationTestRegion, other)
		require.ErrorIs(t, err, native.ErrNotFound, other)
		require.ErrorIs(t, h.Delete(t.Context(), registrationTestRegion, other), native.ErrNotFound, other)
	}
	require.Zero(t, api.calls["DeregisterNamespace"], "a workgroup of another namespace is never deregistered")
	require.ErrorIs(t, h.Delete(t.Context(), registrationTestRegion, "missing|"+registrationConsumer), native.ErrNotFound, "AWS has no such cluster")
	_, err = h.Read(t.Context(), registrationTestRegion, id)
	require.NoError(t, err)
}

// Test_NamespaceRegistration_OtherShares checks that Read finds only the association of the identifier's consumer
// in the namespace's INTERNAL data share, not a consumer of another data share that looks like one, nor another
// consumer's registration.
func Test_NamespaceRegistration_OtherShares(t *testing.T) {
	api := newRegistrationAPI()
	h := NewNamespaceRegistration(api, api)
	_, err := h.Read(t.Context(), registrationTestRegion, "analytics|analytics-wg|"+registrationOtherConsumer)
	require.ErrorIs(t, err, native.ErrNotFound, "the consumer of another data share")

	id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.NoError(t, err)
	registrationProgress(t, h, id, native.OperationCreate)
	_, err = h.Read(t.Context(), registrationTestRegion, "analytics|analytics-wg|111122223333")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), registrationTestRegion, "sales|sales-wg|"+registrationConsumer)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_registrationState(t *testing.T) {
	for _, tt := range []struct {
		status     types.DataShareStatus
		registered bool
		pending    bool
	}{
		{types.DataShareStatusAuthorized, true, false},
		{types.DataShareStatusActive, true, false},
		{types.DataShareStatusAvailable, true, false},
		{types.DataShareStatusPendingAuthorization, false, true},
		{types.DataShareStatusDeauthorized, false, false},
		{types.DataShareStatusRejected, false, false},
		{"", false, false},
	} {
		s := registrationState{status: tt.status}
		require.Equal(t, tt.registered, s.registered(), tt.status)
		require.Equal(t, tt.pending, s.pending(), tt.status)
	}
}

// Test_NamespaceRegistration_Pending checks that a create waits for an association AWS is still authorizing, and a
// delete for one it still lists, and that one AWS ended reads as absent.
func Test_NamespaceRegistration_Pending(t *testing.T) {
	for _, tt := range []struct {
		state registrationState
		op    native.Operation
		want  native.Progress
		err   error
	}{
		{registrationState{status: types.DataShareStatusPendingAuthorization}, native.OperationCreate, native.Progress{State: "PENDING_AUTHORIZATION", Waiting: "AUTHORIZED"}, nil},
		{registrationState{status: types.DataShareStatusPendingAuthorization, busy: "cluster modifying"}, native.OperationCreate, native.Progress{State: "cluster modifying", Waiting: "AUTHORIZED"}, nil},
		{registrationState{busy: "cluster modifying"}, native.OperationCreate, native.Progress{State: "cluster modifying", Waiting: "AUTHORIZED"}, nil},
		{registrationState{status: types.DataShareStatusRejected}, native.OperationCreate, native.Progress{}, native.ErrNotFound},
		{registrationState{}, native.OperationCreate, native.Progress{}, native.ErrNotFound},
		{registrationState{status: types.DataShareStatusPendingAuthorization}, native.OperationDelete, native.Progress{State: "PENDING_AUTHORIZATION", Waiting: "deregistered"}, nil},
		{registrationState{status: types.DataShareStatusDeauthorized}, native.OperationDelete, native.Progress{}, native.ErrNotFound},
	} {
		progress, err := tt.state.progress(tt.op)
		require.Equal(t, tt.err, err, "%+v", tt.state)
		require.Equal(t, tt.want, progress, "%+v", tt.state)
	}
}

// Test_NamespaceRegistration_NamespaceBusy checks that a create waits while the serverless namespace is MODIFYING
// and its workgroup is free, as AWS reported in a recording.
func Test_NamespaceRegistration_NamespaceBusy(t *testing.T) {
	api := newRegistrationAPI()
	h := NewNamespaceRegistration(api, api)
	id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.NoError(t, err)
	registrationProgress(t, h, id, native.OperationCreate)
	api.held[registrationNamespaceType+"/analytics"] = true
	progress, err := h.Progress(t.Context(), registrationTestRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "namespace MODIFYING", Waiting: "available"}, progress)
}

// registrationStuck is an API whose cluster reports a status it may keep, such as storage-full.
type registrationStuck struct {
	*registrationFake
	status string
}

func (r registrationStuck) DescribeClusters(ctx context.Context, in *redshift.DescribeClustersInput, opts ...func(*redshift.Options)) (*redshift.DescribeClustersOutput, error) {
	out, err := r.registrationFake.DescribeClusters(ctx, in, opts...)
	if err == nil {
		out.Clusters[0].ClusterStatus = aws.String(r.status)
	}
	return out, err
}

// Test_NamespaceRegistration_ClusterStates checks that a create waits for a cluster only in a state that ends on its
// own, so a cluster that stays storage-full or paused fails the deploy instead of waiting until the timeout.
func Test_NamespaceRegistration_ClusterStates(t *testing.T) {
	id := "warehouse|" + registrationConsumer
	for status, want := range map[string]native.Progress{
		"rebooting":                  {State: "cluster rebooting", Waiting: "AUTHORIZED"},
		"resizing":                   {State: "cluster resizing", Waiting: "AUTHORIZED"},
		"storage-full":               {},
		"paused":                     {},
		"hardware-failure":           {},
		"available, prep-for-resize": {},
		"incompatible-network":       {},
		"available":                  {},
	} {
		api := registrationStuck{newRegistrationAPI(), status}
		h := NewNamespaceRegistration(api, api)
		progress, err := h.Progress(t.Context(), registrationTestRegion, id, native.OperationCreate)
		if want == (native.Progress{}) {
			require.ErrorIs(t, err, native.ErrNotFound, "a registration AWS doesn't list while the cluster is %s is gone", status)
			continue
		}
		require.NoError(t, err, status)
		require.Equal(t, want, progress, status)
	}
}

// registrationEmpty is an API whose reads of a cluster or namespace return nothing, without an error.
type registrationEmpty struct {
	*registrationFake
}

func (e registrationEmpty) DescribeClusters(ctx context.Context, in *redshift.DescribeClustersInput, opts ...func(*redshift.Options)) (*redshift.DescribeClustersOutput, error) {
	return &redshift.DescribeClustersOutput{}, nil
}

func (e registrationEmpty) GetNamespace(ctx context.Context, in *redshiftserverless.GetNamespaceInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.GetNamespaceOutput, error) {
	return &redshiftserverless.GetNamespaceOutput{}, nil
}

func Test_NamespaceRegistration_EmptyReads(t *testing.T) {
	api := registrationEmpty{newRegistrationAPI()}
	h := NewNamespaceRegistration(api, api)
	for _, identifier := range []string{"warehouse|" + registrationConsumer, "analytics|analytics-wg|" + registrationConsumer} {
		_, err := h.Read(t.Context(), registrationTestRegion, identifier)
		require.ErrorIs(t, err, native.ErrNotFound, identifier)
	}
}

func Test_NamespaceRegistration_Errors(t *testing.T) {
	api := newRegistrationAPI()
	h := NewNamespaceRegistration(api, api)
	var failure *native.Failure
	id, err := h.Identifier(map[string]any{"NamespaceIdentifier": analyticsProps()["NamespaceIdentifier"]})
	require.NoError(t, err)
	require.Empty(t, id, "a template without the consumer gives no identifier")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: serverlessRegistration("analytics", "analytics-wg", "consumer")})
	require.ErrorAs(t, err, &failure)
	require.Zero(t, api.calls["GetWorkgroup"], "an invalid value is never sent")
	for _, identifier := range []string{"", "a", "a|b|c|d", "analytics|analytics-wg|consumer", "Warehouse|" + registrationConsumer} {
		_, err := h.Read(t.Context(), registrationTestRegion, identifier)
		require.ErrorAs(t, err, &failure, identifier)
		require.ErrorAs(t, h.Delete(t.Context(), registrationTestRegion, identifier), &failure, identifier)
		_, err = h.Progress(t.Context(), registrationTestRegion, identifier, native.OperationCreate)
		require.ErrorAs(t, err, &failure, identifier)
	}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: map[string]any{"ConsumerIdentifiers": []any{registrationConsumer}}})
	require.ErrorAs(t, err, &failure, "a create needs every value")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: provisionedRegistration("missing", registrationConsumer)})
	require.ErrorContains(t, err, registrationClusterNotFound, "AWS's error is returned as it is; the real SDK's is a 404, which awsapi.Rejected reports as rejected")
	_, err = h.List(t.Context(), registrationTestRegion)
	require.Error(t, err)

	serverless, provisioned := "analytics|analytics-wg|"+registrationConsumer, "warehouse|"+registrationConsumer
	for method, identifier := range map[string]string{
		"GetWorkgroup":                  serverless,
		"GetNamespace":                  serverless,
		"DescribeClusters":              provisioned,
		"DescribeDataSharesForProducer": provisioned,
	} {
		api.errs[method] = registrationFault("Throttling", "slow down")
		_, err = h.Read(t.Context(), registrationTestRegion, identifier)
		require.ErrorContains(t, err, "Throttling", method)
		delete(api.errs, method)
	}
	api.errs["GetWorkgroup"] = registrationFault("Throttling", "slow down")
	_, err = h.Progress(t.Context(), registrationTestRegion, serverless, native.OperationCreate)
	require.ErrorContains(t, err, "Throttling")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.ErrorContains(t, err, "Throttling")
	require.ErrorContains(t, h.Delete(t.Context(), registrationTestRegion, "analytics|analytics-wg|"+registrationConsumer), "Throttling")
}

// firstResponseLost returns an AWS config whose first call reaches AWS but loses its response with a connection
// error, as a call AWS applied can, and whose later calls get AWS's answer, an XML error with the code. calls counts
// the calls sent.
func firstResponseLost(t *testing.T, code string) (aws.Config, *int) {
	calls := 0
	transport := roundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
		}
		body := `<ErrorResponse><Error><Type>Sender</Type><Code>` + code + `</Code><Message>after a retry</Message></Error><RequestId>1</RequestId></ErrorResponse>`
		return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"text/xml"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	return aws.Config{
		Region:      registrationTestRegion,
		Credentials: credentials.NewStaticCredentialsProvider("AKIATEST", "test", ""),
		HTTPClient:  &http.Client{Transport: transport},
	}, &calls
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Test_NamespaceRegistration_CreateRejected checks that a registration AWS rejected, including one rejected because
// the namespace is busy, returns no identifier, because AWS made nothing.
func Test_NamespaceRegistration_CreateRejected(t *testing.T) {
	api := newRegistrationAPI()
	h := NewNamespaceRegistration(api, api)
	api.errs["RegisterNamespace"] = &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusBadRequest}},
		Err:      registrationFault(registrationInvalid, "The consumer identifier isn't valid."),
	}
	id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.Error(t, err)
	require.Empty(t, id)
	api.errs["RegisterNamespace"] = registrationFault(registrationBusy, "busy")
	id, err = h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: analyticsProps()})
	require.Error(t, err)
	require.Empty(t, id)
}

// Test_NamespaceRegistration_LostResponse checks that a register or deregister AWS may have applied, whose response
// was lost, is never reported as rejected. The SDK would send it again and AWS would answer InvalidClusterState,
// because the first call left the cluster busy, which would drop the record of a registration AWS made.
func Test_NamespaceRegistration_LostResponse(t *testing.T) {
	cfg, calls := firstResponseLost(t, registrationBusy)
	h := NewNamespaceRegistration(redshift.NewFromConfig(cfg), nil)
	id, err := h.Create(t.Context(), native.CreateRequest{Region: registrationTestRegion, Properties: provisionedRegistration("warehouse", registrationConsumer)})
	require.Error(t, err)
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "a lost response is an unknown outcome, not %v", failure)
	require.Equal(t, 1, *calls, "the SDK doesn't send a registration again")
	require.Equal(t, "warehouse|"+registrationConsumer, id, "AWS may have made the registration, so Create returns its identifier")

	cfg, calls = firstResponseLost(t, registrationBusy)
	h = NewNamespaceRegistration(redshift.NewFromConfig(cfg), nil)
	err = h.Delete(t.Context(), registrationTestRegion, "warehouse|"+registrationConsumer)
	require.Error(t, err)
	require.False(t, errors.As(err, &failure), "a lost response is an unknown outcome, not %v", failure)
	require.Equal(t, 1, *calls, "the SDK doesn't send a deregistration again")
}

// allResponsesLost returns an AWS config whose calls all reach AWS but lose their responses, and whose SDK sends
// each call once.
func allResponsesLost() aws.Config {
	return aws.Config{
		Region:           registrationTestRegion,
		Credentials:      credentials.NewStaticCredentialsProvider("AKIATEST", "test", ""),
		RetryMaxAttempts: 1,
		HTTPClient: &http.Client{Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
		})},
	}
}
