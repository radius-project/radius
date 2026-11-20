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

package opensearch

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func newEndpointHandler() (*VpcEndpoint, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewVpcEndpoint(api, api), api
}

// wait polls the progress of op until it is done, and fails the test if it never is.
func wait(t *testing.T, h native.Handler, identifier string, op native.Operation) native.Progress {
	t.Helper()
	for range 10 {
		p, err := h.Progress(t.Context(), region, identifier, op)
		if op == native.OperationDelete && errors.Is(err, native.ErrNotFound) {
			return native.Progress{Done: true}
		}
		require.NoError(t, err)
		if p.Done {
			return p
		}
	}
	require.FailNow(t, "the operation never finished")
	return native.Progress{}
}

// createEndpointWith creates an endpoint with the handler and waits until it is active.
func createEndpointWith(t *testing.T, h *VpcEndpoint, properties map[string]any, clientToken string) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: clientToken})
	require.NoError(t, err)
	require.Nil(t, wait(t, h, id, native.OperationCreate).Failure)
	return id
}

// updateRequest returns the update of the endpoint from what AWS has to desired, with the patch UCP computes.
func updateRequest(t *testing.T, h *VpcEndpoint, id string, desired map[string]any) native.UpdateRequest {
	t.Helper()
	current, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return native.UpdateRequest{Region: region, Identifier: id, Current: current, Desired: desired, Patch: patch}
}

func requireConflict(t *testing.T, err error) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict, "%v", err)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "dependsOn")
}

// requireRejected checks that err is a failure AWS applied none of: a rejected client failure, or a client fault
// from the fake, which UCP takes as rejected. The real SDK's 4xx errors become rejected failures (see the replay
// tests).
func requireRejected(t *testing.T, err error) {
	t.Helper()
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		require.Equal(t, smithy.FaultClient, apiErr.ErrorFault(), "%v", err)
		return
	}
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, "%v", err)
	require.False(t, failure.Conflict, "%v", err)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
}

// Test_VpcEndpoint_FirstLook checks that a new endpoint isn't active the first time UCP reads it, and has no DNS name
// until it is.
func Test_VpcEndpoint_FirstLook(t *testing.T) {
	h, _ := newEndpointHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), nil), ClientToken: "t"})
	require.NoError(t, err)
	p, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, p.Done)
	require.Equal(t, "ACTIVE", p.Waiting)
	properties, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.NotContains(t, properties, "Endpoint", "AWS sets the DNS name once the endpoint is active")

	wait(t, h, id, native.OperationCreate)
	properties, err = h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", properties["Status"])
	require.NotEmpty(t, properties["Endpoint"])
	require.Equal(t, map[string]any{
		"SubnetIds": list("subnet-a"), "SecurityGroupIds": list("sg-default-vpc-1"), "VPCId": "vpc-1", "AvailabilityZones": list("us-west-2a"),
	}, properties["VpcOptions"])
}

// Test_VpcEndpoint_Resend checks that a create sent again with the record's client token returns the first endpoint,
// even while AWS is still creating it and its domain is busy, and that one sent with other values is rejected and
// makes no second endpoint.
func Test_VpcEndpoint_Resend(t *testing.T) {
	h, api := newEndpointHandler()
	request := native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), list("sg-web")), ClientToken: "token-1"}
	first, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Len(t, api.store.List(vpcEndpointType), 1)

	changed := request
	changed.Properties = endpointTemplate(list("subnet-b"), list("sg-web"))
	_, err = h.Create(t.Context(), changed)
	requireRejected(t, err)
	require.Len(t, api.store.List(vpcEndpointType), 1, "a resend with other values makes no second endpoint")
}

// Test_VpcEndpoint_DomainBusy checks that a create or update while AWS is changing the domain, or another endpoint of
// it, is a conflict that sends nothing more, and that the next deploy succeeds once AWS is done.
func Test_VpcEndpoint_DomainBusy(t *testing.T) {
	h, api := newEndpointHandler()
	api.processing["logs"] = 3
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), nil), ClientToken: "a"})
	requireConflict(t, err)
	require.Empty(t, api.store.List(vpcEndpointType))

	delete(api.processing, "logs")
	first, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), nil), ClientToken: "a"})
	require.NoError(t, err)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-b"), nil), ClientToken: "b"})
	requireConflict(t, err)
	require.Len(t, api.store.List(vpcEndpointType), 1, "a second endpoint of a domain waits for the first")

	wait(t, h, first, native.OperationCreate)
	second := createEndpointWith(t, h, endpointTemplate(list("subnet-b"), nil), "b")

	api.processing["logs"] = 3
	err = h.Update(t.Context(), updateRequest(t, h, second, endpointTemplate(list("subnet-a"), nil)))
	requireConflict(t, err)
}

// Test_VpcEndpoint_ChangingEndpoint checks that an update or delete of an endpoint AWS is still changing is a
// conflict, made before any call that changes AWS, and that a delete of an endpoint AWS is already deleting succeeds
// without a call.
func Test_VpcEndpoint_ChangingEndpoint(t *testing.T) {
	h, api := newEndpointHandler()
	api.hold = true
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), nil), ClientToken: "a"})
	require.NoError(t, err)
	current, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	desired := endpointTemplate(list("subnet-a", "subnet-b"), nil)
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	requireConflict(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Current: current, Desired: desired, Patch: patch}))
	requireConflict(t, h.Delete(t.Context(), region, id))
	require.Zero(t, api.calls["UpdateVpcEndpoint"]+api.calls["DeleteVpcEndpoint"])

	api.hold = false
	wait(t, h, id, native.OperationCreate)
	require.NoError(t, h.Delete(t.Context(), region, id))
	require.NoError(t, h.Delete(t.Context(), region, id), "AWS is already deleting it")
	require.Equal(t, 1, api.calls["DeleteVpcEndpoint"])
	wait(t, h, id, native.OperationDelete)
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound)
}

// Test_VpcEndpoint_SecurityGroups checks what an update sends for security groups: the template's, the VPC's default
// group when the template removes them, and AWS's current groups when the template never set them, even ones changed
// outside Radius.
func Test_VpcEndpoint_SecurityGroups(t *testing.T) {
	h, api := newEndpointHandler()
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a"), list("sg-web", "sg-db")), "a")

	require.NoError(t, h.Update(t.Context(), updateRequest(t, h, id, endpointTemplate(list("subnet-a"), nil))))
	wait(t, h, id, native.OperationUpdate)
	properties, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, list("sg-default-vpc-1"), properties["VpcOptions"].(map[string]any)["SecurityGroupIds"])
	require.Equal(t, 1, api.calls["DescribeSecurityGroups"])

	// Outside Radius, someone sets other groups; a deploy that never set them and changes only the subnets keeps them.
	_, err = api.UpdateVpcEndpoint(t.Context(), &opensearch.UpdateVpcEndpointInput{VpcEndpointId: aws.String(id), VpcOptions: &types.VPCOptions{
		SubnetIds: []string{"subnet-a"}, SecurityGroupIds: []string{"sg-web"},
	}})
	require.NoError(t, err)
	wait(t, h, id, native.OperationUpdate)
	request := updateRequest(t, h, id, endpointTemplate(list("subnet-a", "subnet-b"), nil))
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationAdd, Path: "/VpcOptions/SubnetIds/-", Value: "subnet-b"}}
	require.NoError(t, h.Update(t.Context(), request))
	wait(t, h, id, native.OperationUpdate)
	properties, err = h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, list("sg-web"), properties["VpcOptions"].(map[string]any)["SecurityGroupIds"])
	require.Equal(t, list("subnet-a", "subnet-b"), properties["VpcOptions"].(map[string]any)["SubnetIds"])
	require.Equal(t, 1, api.calls["DescribeSecurityGroups"], "groups the template never set need no lookup")
}

// Test_VpcEndpoint_RemovedSubnets checks that subnets the template removes are sent as an empty list, so AWS's
// rejection shows, rather than kept.
func Test_VpcEndpoint_RemovedSubnets(t *testing.T) {
	h, _ := newEndpointHandler()
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a"), nil), "a")
	desired := map[string]any{"DomainArn": domainArn("logs"), "VpcOptions": map[string]any{}}
	request := updateRequest(t, h, id, desired)
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/VpcOptions/SubnetIds"}}
	requireRejected(t, h.Update(t.Context(), request))
}

// fixedGroups is an EC2 API that returns groups for every DescribeSecurityGroups.
type fixedGroups struct {
	groups []ec2types.SecurityGroup
	err    error
}

func (f fixedGroups) DescribeSecurityGroups(context.Context, *awsec2.DescribeSecurityGroupsInput, ...func(*awsec2.Options)) (*awsec2.DescribeSecurityGroupsOutput, error) {
	return &awsec2.DescribeSecurityGroupsOutput{SecurityGroups: f.groups}, f.err
}

// Test_VpcEndpoint_DefaultGroupNotFound checks that an update that can't find exactly one default group sends
// nothing.
func Test_VpcEndpoint_DefaultGroupNotFound(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	id := createEndpointWith(t, NewVpcEndpoint(api, api), endpointTemplate(list("subnet-a"), list("sg-web")), "a")
	for _, ec2 := range []fixedGroups{
		{},
		{groups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-1")}, {GroupId: aws.String("sg-2")}}},
		{err: errors.New("denied")},
	} {
		h := NewVpcEndpoint(api, ec2)
		require.Error(t, h.Update(t.Context(), updateRequest(t, h, id, endpointTemplate(list("subnet-a"), nil))))
	}
	require.Zero(t, api.calls["UpdateVpcEndpoint"])

	_, err := NewVpcEndpoint(api, api).defaultSecurityGroup(t.Context(), region, "")
	require.ErrorContains(t, err, "no VPC")
}

// Test_VpcEndpoint_CreateFailed checks that an endpoint AWS failed to create fails the create, names what to do, and
// can be deleted.
func Test_VpcEndpoint_CreateFailed(t *testing.T) {
	h, api := newEndpointHandler()
	api.failCreate = true
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), nil), ClientToken: "a"})
	require.NoError(t, err)
	p := wait(t, h, id, native.OperationCreate)
	require.NotNil(t, p.Failure)
	require.Contains(t, p.Failure.Message, id)
	require.Contains(t, p.Failure.Message, "CREATE_FAILED")
	require.Contains(t, p.Failure.Message, "Delete the resource first", "a redeploy alone returns the same failed endpoint")
	require.NoError(t, h.Delete(t.Context(), region, id))
	wait(t, h, id, native.OperationDelete)
}

// Test_VpcEndpoint_DeleteFailed checks that a delete AWS reports as failed ends the wait with a failure.
func Test_VpcEndpoint_DeleteFailed(t *testing.T) {
	h, api := newEndpointHandler()
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a"), nil), "a")
	api.status[id] = types.VpcEndpointStatusDeleteFailed
	p, err := h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.Contains(t, p.Failure.Message, "Delete again")
	require.NoError(t, h.Delete(t.Context(), region, id), "a failed delete can be sent again")
}

// describeOnly is an OpenSearch API whose DescribeVpcEndpoints returns out.
type describeOnly struct {
	API
	out *opensearch.DescribeVpcEndpointsOutput
}

func (d describeOnly) DescribeVpcEndpoints(context.Context, *opensearch.DescribeVpcEndpointsInput, ...func(*opensearch.Options)) (*opensearch.DescribeVpcEndpointsOutput, error) {
	return d.out, nil
}

// Test_VpcEndpoint_Describe checks the answers of DescribeVpcEndpoints that aren't the endpoint: not found, a server
// error, and an answer that names the endpoint nowhere, which isn't read as not found.
func Test_VpcEndpoint_Describe(t *testing.T) {
	const id = "aos-0000000000000009"
	for _, tc := range []struct {
		out  *opensearch.DescribeVpcEndpointsOutput
		want string
	}{
		{out: &opensearch.DescribeVpcEndpointsOutput{VpcEndpointErrors: []types.VpcEndpointError{{VpcEndpointId: aws.String(id), ErrorCode: types.VpcEndpointErrorCodeEndpointNotFound}}}},
		{out: &opensearch.DescribeVpcEndpointsOutput{VpcEndpointErrors: []types.VpcEndpointError{{VpcEndpointId: aws.String(id), ErrorCode: types.VpcEndpointErrorCodeServerError, ErrorMessage: aws.String("boom")}}}, want: "boom"},
		{out: &opensearch.DescribeVpcEndpointsOutput{VpcEndpointErrors: []types.VpcEndpointError{{VpcEndpointId: aws.String("aos-other"), ErrorCode: types.VpcEndpointErrorCodeEndpointNotFound}}}, want: "returned nothing"},
	} {
		h := NewVpcEndpoint(describeOnly{out: tc.out}, nil)
		_, err := h.Read(t.Context(), region, id)
		if tc.want == "" {
			require.ErrorIs(t, err, native.ErrNotFound)
			continue
		}
		require.ErrorContains(t, err, tc.want)
		require.NotErrorIs(t, err, native.ErrNotFound)
	}
}

// Test_VpcEndpoint_BadInput checks that an identifier that isn't an endpoint ID, and a template whose values have the
// wrong type, fail as client failures before any AWS call.
func Test_VpcEndpoint_BadInput(t *testing.T) {
	h, api := newEndpointHandler()
	for _, identifier := range []string{"vpce-1", "aos", "aos-a/b"} {
		_, err := h.Read(t.Context(), region, identifier)
		requireRejected(t, err)
		requireRejected(t, h.Delete(t.Context(), region, identifier))
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		requireRejected(t, err)
	}
	for _, properties := range []map[string]any{
		{"VpcOptions": map[string]any{"SubnetIds": list("subnet-a")}},
		{"DomainArn": domainArn("logs"), "VpcOptions": "subnet-a"},
		{"DomainArn": domainArn("logs"), "VpcOptions": map[string]any{"SubnetIds": "subnet-a"}},
		{"DomainArn": domainArn("logs"), "VpcOptions": map[string]any{"SubnetIds": list("subnet-a"), "SecurityGroupIds": []any{1}}},
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: "a"})
		requireRejected(t, err)
	}
	require.Zero(t, api.calls["CreateVpcEndpoint"]+api.calls["DescribeVpcEndpoints"])
}

// Test_VpcEndpoint_List checks that List reads every page and leaves out endpoints AWS is deleting.
func Test_VpcEndpoint_List(t *testing.T) {
	h, _ := newEndpointHandler()
	ids := []string{}
	for i, name := range []string{"logs", "metrics", "logs"} {
		properties := endpointTemplate(list("subnet-a"), nil)
		properties["DomainArn"] = domainArn(name)
		ids = append(ids, createEndpointWith(t, h, properties, string(rune('a'+i))))
	}
	require.NoError(t, h.Delete(t.Context(), region, ids[1]))
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	got := []string{}
	for _, r := range resources {
		got = append(got, r.Identifier)
		require.Equal(t, r.Identifier, r.Properties["VpcEndpointId"])
	}
	require.ElementsMatch(t, []string{ids[0], ids[2]}, got)
}

// Test_VpcEndpoint_AWSErrors checks that an error from a read AWS fails is returned, and nothing that changes AWS is
// sent after it.
func Test_VpcEndpoint_AWSErrors(t *testing.T) {
	h, api := newEndpointHandler()
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a"), nil), "a")
	request := updateRequest(t, h, id, endpointTemplate(list("subnet-b"), nil))
	boom := errors.New("boom")

	api.errs["DescribeVpcEndpoints"] = boom
	_, err := h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, boom)
	requireRejectedBefore(t, h.Update(t.Context(), request), "boom")
	requireRejectedBefore(t, h.Delete(t.Context(), region, id), "boom")
	_, err = h.List(t.Context(), region)
	require.ErrorIs(t, err, boom)
	require.Zero(t, api.calls["UpdateVpcEndpoint"]+api.calls["DeleteVpcEndpoint"])

	delete(api.errs, "DescribeVpcEndpoints")
	api.errs["ListVpcEndpoints"] = boom
	_, err = h.List(t.Context(), region)
	require.ErrorIs(t, err, boom)

	bad := request
	bad.Desired = map[string]any{"VpcOptions": map[string]any{"SubnetIds": "subnet-b"}}
	requireRejected(t, h.Update(t.Context(), bad))
}

// Test_VpcEndpoint_UnsetSubnets checks that an update whose template leaves SubnetIds out, without removing them,
// keeps AWS's subnets.
func Test_VpcEndpoint_UnsetSubnets(t *testing.T) {
	h, _ := newEndpointHandler()
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a", "subnet-b"), list("sg-web")), "a")
	request := updateRequest(t, h, id, map[string]any{"DomainArn": domainArn("logs"), "VpcOptions": map[string]any{"SecurityGroupIds": list("sg-db")}})
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/VpcOptions/SecurityGroupIds/0", Value: "sg-db"}}
	require.NoError(t, h.Update(t.Context(), request))
	wait(t, h, id, native.OperationUpdate)
	properties, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, list("subnet-a", "subnet-b"), properties["VpcOptions"].(map[string]any)["SubnetIds"])
	require.Equal(t, list("sg-db"), properties["VpcOptions"].(map[string]any)["SecurityGroupIds"])
}

// odd is an OpenSearch API that answers in ways AWS's docs don't promise: a create that returns no endpoint, an
// endpoint without VpcOptions, and a describe that reports an endpoint the list showed as deleting.
type odd struct {
	*fakeAPI
	describeStatus types.VpcEndpointStatus
}

func (o *odd) CreateVpcEndpoint(context.Context, *opensearch.CreateVpcEndpointInput, ...func(*opensearch.Options)) (*opensearch.CreateVpcEndpointOutput, error) {
	return &opensearch.CreateVpcEndpointOutput{}, nil
}

func (o *odd) DescribeVpcEndpoints(ctx context.Context, in *opensearch.DescribeVpcEndpointsInput, opts ...func(*opensearch.Options)) (*opensearch.DescribeVpcEndpointsOutput, error) {
	out, err := o.fakeAPI.DescribeVpcEndpoints(ctx, in, opts...)
	for i := range out.VpcEndpoints {
		out.VpcEndpoints[i].VpcOptions = nil
		if o.describeStatus != "" {
			out.VpcEndpoints[i].Status = o.describeStatus
		}
	}
	return out, err
}

func Test_VpcEndpoint_OddAnswers(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	id := createEndpointWith(t, NewVpcEndpoint(api, api), endpointTemplate(list("subnet-a"), nil), "a")
	o := &odd{fakeAPI: api}
	h := NewVpcEndpoint(o, api)

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: endpointTemplate(list("subnet-a"), nil), ClientToken: "b"})
	require.ErrorContains(t, err, "returned no ID")

	// With no VpcOptions from AWS, an update that removes the groups can't find the VPC, and sends nothing.
	request := native.UpdateRequest{Region: region, Identifier: id, Desired: endpointTemplate(list("subnet-a"), nil),
		Patch: jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/VpcOptions/SecurityGroupIds"}}}
	require.ErrorContains(t, h.Update(t.Context(), request), "no VPC")
	require.Zero(t, api.calls["UpdateVpcEndpoint"])

	o.describeStatus = types.VpcEndpointStatusDeleting
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources, "an endpoint AWS started deleting after the list is left out")
}

// requireRejectedBefore checks that err is a failure from before any change, so UCP drops the pending values, and
// that it names cause.
func requireRejectedBefore(t *testing.T, err error, cause string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, "%v", err)
	require.Contains(t, failure.Message, cause)
}

// Test_VpcEndpoint_FailsBeforeChange checks that a read that fails before an update or delete sends anything is
// reported as rejected, so UCP doesn't count the update's values as possibly applied.
func Test_VpcEndpoint_FailsBeforeChange(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewVpcEndpoint(api, api)
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a"), list("sg-web")), "a")
	removeGroups := updateRequest(t, h, id, endpointTemplate(list("subnet-a"), nil))

	requireRejectedBefore(t, NewVpcEndpoint(api, fixedGroups{err: errors.New("ec2 down")}).Update(t.Context(), removeGroups), "ec2 down")
	api.errs["DescribeVpcEndpoints"] = errors.New("describe down")
	requireRejectedBefore(t, h.Update(t.Context(), removeGroups), "describe down")
	requireRejectedBefore(t, h.Delete(t.Context(), region, id), "describe down")
	require.Zero(t, api.calls["UpdateVpcEndpoint"]+api.calls["DeleteVpcEndpoint"])
}

// serverErrors is an OpenSearch API whose DescribeVpcEndpoints reports every endpoint as a server error.
type serverErrors struct {
	*fakeAPI
}

func (s serverErrors) DescribeVpcEndpoints(_ context.Context, in *opensearch.DescribeVpcEndpointsInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeVpcEndpointsOutput, error) {
	out := &opensearch.DescribeVpcEndpointsOutput{}
	for _, id := range in.VpcEndpointIds {
		out.VpcEndpointErrors = append(out.VpcEndpointErrors, types.VpcEndpointError{VpcEndpointId: aws.String(id), ErrorCode: types.VpcEndpointErrorCodeServerError, ErrorMessage: aws.String("try later")})
	}
	return out, nil
}

// Test_VpcEndpoint_ListServerError checks that List fails, rather than leaving an endpoint out, when AWS can't
// describe it.
func Test_VpcEndpoint_ListServerError(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	createEndpointWith(t, NewVpcEndpoint(api, api), endpointTemplate(list("subnet-a"), nil), "a")
	_, err := NewVpcEndpoint(serverErrors{api}, api).List(t.Context(), region)
	require.ErrorContains(t, err, "try later")
}

// Test_VpcEndpoint_UpdateFailed checks that an update AWS reports as failed tells the user to deploy again, which sends
// the update again.
func Test_VpcEndpoint_UpdateFailed(t *testing.T) {
	h, api := newEndpointHandler()
	id := createEndpointWith(t, h, endpointTemplate(list("subnet-a"), nil), "a")
	api.status[id] = types.VpcEndpointStatusUpdateFailed
	p, err := h.Progress(t.Context(), region, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.Contains(t, p.Failure.Message, "UPDATE_FAILED")
	require.Contains(t, p.Failure.Message, "deploy again")
	require.NotContains(t, p.Failure.Message, "Delete the resource first")
	require.NoError(t, h.Update(t.Context(), updateRequest(t, h, id, endpointTemplate(list("subnet-b"), nil))), "a failed update can be sent again")
}

// Test_VpcEndpoint_EmptyLists checks that a template with an empty SubnetIds or SecurityGroupIds is rejected before any
// AWS call. AWS needs a subnet, and an empty security group list reads back as the VPC's default group, so it would
// update the endpoint on every deploy.
func Test_VpcEndpoint_EmptyLists(t *testing.T) {
	h, api := newEndpointHandler()
	for _, properties := range []map[string]any{
		endpointTemplate([]any{}, nil),
		endpointTemplate(list("subnet-a"), []any{}),
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties, ClientToken: "a"})
		requireRejected(t, err)
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, "rejected by the handler, not by AWS")
	}
	require.Zero(t, api.calls["CreateVpcEndpoint"])
}
