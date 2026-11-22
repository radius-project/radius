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

package redshiftserverless

import (
	"context"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const endpointFile = "aws-redshiftserverless-endpointaccess.json"

// ownEndpoint is an endpoint of a workgroup of this account.
var ownEndpoint = with(endpoint, map[string]any{"WorkgroupName": "analytics", "OwnerAccount": nil})

// createdEndpoint creates the endpoint properties give and reads it until it is ACTIVE.
func createdEndpoint(t *testing.T, h *EndpointAccess, properties map[string]any) {
	t.Helper()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	waitActive(t, h, native.OperationCreate)
}

// waitActive calls Progress until op is done, and fails if it is done the first time.
func waitActive(t *testing.T, h *EndpointAccess, op native.Operation) {
	t.Helper()
	progress, err := h.Progress(t.Context(), region, "reads", op)
	require.NoError(t, err)
	require.False(t, progress.Done, "the endpoint is not ACTIVE when the code first looks")
	require.Equal(t, endpointActive, progress.Waiting)
	progress, err = h.Progress(t.Context(), region, "reads", op)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_EndpointAccess_Create(t *testing.T) {
	api := newAPI()
	h := NewEndpointAccess(api)
	createdEndpoint(t, h, with(ownEndpoint, map[string]any{"VpcSecurityGroupIds": nil}))
	properties, err := h.Read(t.Context(), region, "reads")
	require.NoError(t, err)
	require.Equal(t, []any{defaultGroup}, properties["VpcSecurityGroupIds"], "AWS gives an endpoint without groups the VPC's default group")
	require.NotContains(t, properties, "OwnerAccount", "AWS doesn't return it")
	require.EqualValues(t, 5439, properties["Port"])

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: ownEndpoint})
	require.Contains(t, requireFailure(t, err, native.OutcomeUnknown, false), "already exists", "an earlier attempt may have made the endpoint, so UCP keeps the record")

	api.held["reports"] = true
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(ownEndpoint, map[string]any{"EndpointName": "other", "WorkgroupName": "reports"})})
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, true), "The workgroup reports is busy")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(ownEndpoint, map[string]any{"EndpointName": "other", "WorkgroupName": "missing"})})
	require.ErrorContains(t, err, notFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(ownEndpoint, map[string]any{"EndpointName": "Other"})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
	require.Equal(t, 4, api.calls["CreateEndpointAccess"], "a name AWS wouldn't take is never sent")
}

// Test_EndpointAccess_CreateIgnoresOtherCaseKeys checks that a key in another case, such as workgroupName, can't send
// the create to another workgroup than the one the template names.
func Test_EndpointAccess_CreateIgnoresOtherCaseKeys(t *testing.T) {
	api := newAPI()
	h := NewEndpointAccess(api)
	createdEndpoint(t, h, with(ownEndpoint, map[string]any{"workgroupName": "reports", "subnetIds": []any{"subnet-9"}}))
	stored := api.store.Resource(endpointAccessType, "reads")
	require.Equal(t, "analytics", stored["WorkgroupName"])
	require.Equal(t, []any{"subnet-1", "subnet-2"}, stored["SubnetIds"])
}

func Test_EndpointAccess_Update(t *testing.T) {
	api := newAPI()
	h := NewEndpointAccess(api)
	createdEndpoint(t, h, ownEndpoint)

	require.NoError(t, update(t, h, endpointFile, "reads", ownEndpoint, with(ownEndpoint, map[string]any{"VpcSecurityGroupIds": []any{"sg-3", "sg-1"}})))
	require.Zero(t, api.calls["UpdateEndpointAccess"], "an unchanged template sends nothing")

	changed := with(ownEndpoint, map[string]any{"VpcSecurityGroupIds": []any{"sg-3", "sg-2"}})
	require.NoError(t, update(t, h, endpointFile, "reads", ownEndpoint, changed))
	require.Equal(t, 1, api.calls["UpdateEndpointAccess"])
	waitActive(t, h, native.OperationUpdate)

	// AWS rejects the groups the endpoint already has, so a patch that only reorders them sends nothing.
	err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: with(changed, map[string]any{"VpcSecurityGroupIds": []any{"sg-2", "sg-3"}}), Patch: patchOf(t, "/VpcSecurityGroupIds", []any{"sg-2", "sg-3"})})
	require.NoError(t, err)
	require.Equal(t, 1, api.calls["UpdateEndpointAccess"])

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: with(changed, map[string]any{"VpcSecurityGroupIds": []any{}}), Patch: patchOf(t, "/VpcSecurityGroupIds", []any{})})
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, false), "at least one security group")

	api.status["reads"] = "MODIFYING"
	busy := NewEndpointAccess(&steadyStatus{fakeAPI: api})
	err = busy.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: ownEndpoint, Patch: patchOf(t, "/VpcSecurityGroupIds", []any{"sg-1"})})
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, true), "The endpoint reads is busy")
	delete(api.status, "reads")

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "gone", Desired: ownEndpoint, Patch: patchOf(t, "/VpcSecurityGroupIds", []any{"sg-1"})})
	require.ErrorIs(t, err, native.ErrNotFound)

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: with(ownEndpoint, map[string]any{"VpcSecurityGroupIds": "sg-1"})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
}

func Test_EndpointAccess_Delete(t *testing.T) {
	api := newAPI()
	h := NewEndpointAccess(api)
	require.ErrorIs(t, h.Delete(t.Context(), region, "reads"), native.ErrNotFound)

	createdEndpoint(t, h, ownEndpoint)
	require.NoError(t, h.Delete(t.Context(), region, "reads"))
	progress, err := h.Progress(t.Context(), region, "reads", native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: endpointDeleting, Waiting: "deleted"}, progress)
	_, err = h.Progress(t.Context(), region, "reads", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	createdEndpoint(t, h, ownEndpoint)
	api.status["reads"] = endpointDeleting
	require.NoError(t, h.Delete(t.Context(), region, "reads"), "an endpoint AWS is deleting had a delete reach AWS")
	require.Equal(t, 1, api.calls["DeleteEndpointAccess"])

	createdEndpoint(t, h, ownEndpoint)
	api.status["reads"] = "MODIFYING"
	h = NewEndpointAccess(&steadyStatus{fakeAPI: api})
	require.Contains(t, requireFailure(t, h.Delete(t.Context(), region, "reads"), native.OutcomeRejected, true), "The endpoint reads is busy")
}

// steadyStatus is an API whose reads of an endpoint don't move its status on.
type steadyStatus struct {
	*fakeAPI
}

func (s *steadyStatus) GetEndpointAccess(ctx context.Context, in *redshiftserverless.GetEndpointAccessInput, _ ...func(*redshiftserverless.Options)) (*redshiftserverless.GetEndpointAccessOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	props := s.store.Resource(endpointAccessType, aws.ToString(in.EndpointName))
	e := endpointOf(props, s.status[aws.ToString(in.EndpointName)])
	return &redshiftserverless.GetEndpointAccessOutput{Endpoint: &e}, nil
}

// racingDelete starts deleting the endpoint when the handler's delete call reaches it, as if another delete got there
// first.
type racingDelete struct {
	*fakeAPI
	failGet bool
}

func (r *racingDelete) DeleteEndpointAccess(ctx context.Context, in *redshiftserverless.DeleteEndpointAccessInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteEndpointAccessOutput, error) {
	r.mu.Lock()
	r.status[aws.ToString(in.EndpointName)] = endpointDeleting
	if r.failGet {
		r.errs["GetEndpointAccess"] = fault("ThrottlingException", "slow down")
	}
	r.mu.Unlock()
	return r.fakeAPI.DeleteEndpointAccess(ctx, in, opts...)
}

func Test_EndpointAccess_DeleteRace(t *testing.T) {
	api := newAPI()
	createdEndpoint(t, NewEndpointAccess(api), ownEndpoint)
	require.NoError(t, NewEndpointAccess(&racingDelete{fakeAPI: api}).Delete(t.Context(), region, "reads"), "an endpoint another delete started on is deleted")

	api = newAPI()
	createdEndpoint(t, NewEndpointAccess(api), ownEndpoint)
	require.ErrorContains(t, NewEndpointAccess(&racingDelete{fakeAPI: api, failGet: true}).Delete(t.Context(), region, "reads"), "ThrottlingException")
}

func Test_EndpointAccess_ReadAndList(t *testing.T) {
	api := newAPI()
	h := NewEndpointAccess(api)
	createdEndpoint(t, h, ownEndpoint)
	properties, err := h.Read(t.Context(), region, "reads")
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"NetworkInterfaces": []any{map[string]any{"AvailabilityZone": "us-west-2a", "NetworkInterfaceId": "eni-reads", "PrivateIpAddress": "10.0.0.10", "SubnetId": "subnet-1"}},
		"VpcEndpointId":     "vpce-reads",
		"VpcId":             "vpc-1",
	}, properties["VpcEndpoint"])

	for i := range 120 {
		api.store.Put(endpointAccessType, "e"+strconv.Itoa(i), map[string]any{"EndpointName": "e" + strconv.Itoa(i), "WorkgroupName": "analytics", "SubnetIds": []any{"subnet-1"}})
	}
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 121)

	api.errs["ListEndpointAccess"] = fault("ThrottlingException", "slow down")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
	api.errs["GetEndpointAccess"] = fault("ThrottlingException", "slow down")
	_, err = h.Progress(t.Context(), region, "reads", native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
}

// Test_securityGroups checks that a group AWS is removing isn't one the endpoint has, so an update that removes it
// isn't sent again while AWS removes it.
func Test_securityGroups(t *testing.T) {
	e := &types.EndpointAccess{VpcSecurityGroups: []types.VpcSecurityGroupMembership{
		{VpcSecurityGroupId: aws.String("sg-new"), Status: aws.String("creating")},
		{VpcSecurityGroupId: aws.String("sg-old"), Status: aws.String("deleting")},
		{VpcSecurityGroupId: aws.String("sg-kept"), Status: aws.String("active")},
	}}
	require.Equal(t, []string{"sg-new", "sg-kept"}, securityGroups(e))
	require.True(t, sameSet([]string{"b", "a", "a"}, []string{"a", "b"}))
	require.False(t, sameSet([]string{"a"}, []string{"a", "b"}))
}

// Test_EndpointAccess_EmptyResponses checks that a response without an endpoint is not found.
func Test_EndpointAccess_EmptyResponses(t *testing.T) {
	h := NewEndpointAccess(emptyAPI{newAPI()})
	_, err := h.Read(t.Context(), region, "reads")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = NewUsageLimit(emptyAPI{newAPI()}).Read(t.Context(), region, "limit")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = NewCustomDomainAssociation(emptyAPI{newAPI()}).Read(t.Context(), region, "analytics|db.example.com")
	require.ErrorIs(t, err, native.ErrNotFound)
}

// emptyAPI is an API whose reads return nothing.
type emptyAPI struct {
	*fakeAPI
}

func (emptyAPI) GetEndpointAccess(context.Context, *redshiftserverless.GetEndpointAccessInput, ...func(*redshiftserverless.Options)) (*redshiftserverless.GetEndpointAccessOutput, error) {
	return &redshiftserverless.GetEndpointAccessOutput{}, nil
}

func (emptyAPI) GetUsageLimit(context.Context, *redshiftserverless.GetUsageLimitInput, ...func(*redshiftserverless.Options)) (*redshiftserverless.GetUsageLimitOutput, error) {
	return &redshiftserverless.GetUsageLimitOutput{}, nil
}

func (emptyAPI) GetCustomDomainAssociation(context.Context, *redshiftserverless.GetCustomDomainAssociationInput, ...func(*redshiftserverless.Options)) (*redshiftserverless.GetCustomDomainAssociationOutput, error) {
	return &redshiftserverless.GetCustomDomainAssociationOutput{}, nil
}

// lostUpdate is an API whose UpdateEndpointAccess applies the change and then answers ConflictException, as an SDK
// retry of an update whose first answer was lost gets from an endpoint the first attempt left MODIFYING.
type lostUpdate struct {
	*fakeAPI
}

func (l lostUpdate) UpdateEndpointAccess(ctx context.Context, in *redshiftserverless.UpdateEndpointAccessInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateEndpointAccessOutput, error) {
	if _, err := l.fakeAPI.UpdateEndpointAccess(ctx, in, opts...); err != nil {
		return nil, err
	}
	return l.fakeAPI.UpdateEndpointAccess(ctx, in, opts...)
}

func (l lostUpdate) UpdateCustomDomainAssociation(ctx context.Context, in *redshiftserverless.UpdateCustomDomainAssociationInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateCustomDomainAssociationOutput, error) {
	if _, err := l.fakeAPI.UpdateCustomDomainAssociation(ctx, in, opts...); err != nil {
		return nil, err
	}
	return l.fakeAPI.UpdateCustomDomainAssociation(ctx, in, opts...)
}

// Test_EndpointAccess_UpdateRetried checks that an update whose retry AWS rejects because the first attempt left the
// endpoint MODIFYING is not reported as rejected, so UCP keeps the values AWS is applying.
func Test_EndpointAccess_UpdateRetried(t *testing.T) {
	api := newAPI()
	createdEndpoint(t, NewEndpointAccess(api), ownEndpoint)
	h := NewEndpointAccess(&steadyStatus{fakeAPI: api})
	retried := NewEndpointAccess(lostUpdate{api})
	changed := with(ownEndpoint, map[string]any{"VpcSecurityGroupIds": []any{"sg-2"}})
	err := retried.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: changed, Patch: patchOf(t, "/VpcSecurityGroupIds", []any{"sg-2"})})
	require.NoError(t, err)
	require.Equal(t, []any{"sg-2"}, api.store.Resource(endpointAccessType, "reads")["VpcSecurityGroupIds"])
	progress, err := h.Progress(t.Context(), region, "reads", native.OperationUpdate)
	require.NoError(t, err)
	require.False(t, progress.Done, "UCP waits for the update AWS is applying")
}

// Test_UpdateRetriedUnconfirmed checks that an update AWS answers with ConflictException, whose read to see whether an
// earlier attempt reached AWS fails, has an unknown outcome, so UCP keeps its values as possibly applied.
func Test_UpdateRetriedUnconfirmed(t *testing.T) {
	api := newAPI()
	createdEndpoint(t, NewEndpointAccess(api), ownEndpoint)
	createdDomain(t, NewCustomDomainAssociation(api), domain)
	api.errs["UpdateEndpointAccess"] = fault(conflict, "busy")
	api.errs["UpdateCustomDomainAssociation"] = fault(conflict, "busy")
	failing := &failSecondGet{fakeAPI: api}

	err := NewEndpointAccess(failing).Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "reads", Desired: with(ownEndpoint, map[string]any{"VpcSecurityGroupIds": []any{"sg-2"}}), Patch: patchOf(t, "/VpcSecurityGroupIds", []any{"sg-2"})})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.Contains(t, failure.Message, "ThrottlingException")

	err = NewCustomDomainAssociation(failing).Update(t.Context(), native.UpdateRequest{Region: region, Identifier: domainID, Desired: with(domain, map[string]any{"CustomDomainCertificateArn": certificates[1]}), Patch: patchOf(t, "/CustomDomainCertificateArn", certificates[1])})
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
}

// failSecondGet is an API whose reads of an endpoint fail after the first, and whose reads of an association always fail.
type failSecondGet struct {
	*fakeAPI
	endpointReads int
}

func (f *failSecondGet) GetEndpointAccess(ctx context.Context, in *redshiftserverless.GetEndpointAccessInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.GetEndpointAccessOutput, error) {
	if f.endpointReads++; f.endpointReads > 1 {
		return nil, fault("ThrottlingException", "slow down")
	}
	return f.fakeAPI.GetEndpointAccess(ctx, in, opts...)
}

func (f *failSecondGet) GetCustomDomainAssociation(ctx context.Context, in *redshiftserverless.GetCustomDomainAssociationInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.GetCustomDomainAssociationOutput, error) {
	return nil, fault("ThrottlingException", "slow down")
}

// Test_CreateKeepsTheIdentifierAWSMayHaveUsed checks that a Create whose call AWS may have applied, such as one whose
// answer was lost or that AWS answered "already exists", returns the identifier with its failure, so a failed recording
// registers cleanup for what AWS may have made, and that a Create AWS rejected returns none.
func Test_CreateKeepsTheIdentifierAWSMayHaveUsed(t *testing.T) {
	serverError := &smithy.GenericAPIError{Code: "InternalServerException", Message: "lost", Fault: smithy.FaultServer}
	tests := []struct {
		name    string
		create  func(api *fakeAPI) (string, error)
		prepare func(api *fakeAPI)
		want    string
	}{
		{
			name:    "an endpoint whose answer was lost",
			prepare: func(api *fakeAPI) { api.errs["CreateEndpointAccess"] = serverError },
			create: func(api *fakeAPI) (string, error) {
				return NewEndpointAccess(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: ownEndpoint})
			},
			want: "reads",
		},
		{
			name: "an endpoint AWS already has",
			prepare: func(api *fakeAPI) {
				_, _ = NewEndpointAccess(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: ownEndpoint})
			},
			create: func(api *fakeAPI) (string, error) {
				return NewEndpointAccess(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: ownEndpoint})
			},
			want: "reads",
		},
		{
			name:    "an endpoint of a busy workgroup",
			prepare: func(api *fakeAPI) { api.held["analytics"] = true },
			create: func(api *fakeAPI) (string, error) {
				return NewEndpointAccess(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: ownEndpoint})
			},
		},
		{
			name:    "an endpoint of a missing workgroup",
			prepare: func(api *fakeAPI) {},
			create: func(api *fakeAPI) (string, error) {
				return NewEndpointAccess(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: with(ownEndpoint, map[string]any{"WorkgroupName": "missing"})})
			},
		},
		{
			name:    "an association whose answer was lost",
			prepare: func(api *fakeAPI) { api.errs["CreateCustomDomainAssociation"] = serverError },
			create: func(api *fakeAPI) (string, error) {
				return NewCustomDomainAssociation(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: domain})
			},
			want: domainID,
		},
		{
			name: "an association AWS already has",
			prepare: func(api *fakeAPI) {
				_, _ = NewCustomDomainAssociation(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: domain})
				api.busy["analytics"] = 0
			},
			create: func(api *fakeAPI) (string, error) {
				return NewCustomDomainAssociation(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: domain})
			},
			want: domainID,
		},
		{
			name: "an association whose conflict AWS can't confirm",
			prepare: func(api *fakeAPI) {
				api.errs["CreateCustomDomainAssociation"] = fault(conflict, "busy")
				api.errs["GetCustomDomainAssociation"] = fault("ThrottlingException", "slow down")
			},
			create: func(api *fakeAPI) (string, error) {
				return NewCustomDomainAssociation(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: domain})
			},
			want: domainID,
		},
		{
			name:    "an association with a certificate ACM doesn't have",
			prepare: func(api *fakeAPI) {},
			create: func(api *fakeAPI) (string, error) {
				return NewCustomDomainAssociation(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: with(domain, map[string]any{"CustomDomainCertificateArn": "arn:aws:acm:us-west-2:123456789012:certificate/missing"})})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newAPI()
			tt.prepare(api)
			identifier, err := tt.create(api)
			require.Error(t, err)
			require.Equal(t, tt.want, identifier)
		})
	}
}
