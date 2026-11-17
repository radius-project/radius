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

package ec2

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

var testPolicy = map[string]any{"PolicyDocument": "permit(principal, action, resource);", "ShadowMode": "enabled", "Description": "allow all"}

func Test_ClientVpnEndpoint_CreateWithPolicy(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	request := createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": testPolicy}))

	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"CreateClientVpnEndpoint", "ModifyClientVpnEndpointAuthorizationPolicy"}, api.calls)
	require.Equal(t, id, api.tokens[clientVpnEndpointType+"/"+request.ClientToken], "the create sends the record's client token")

	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, again, "a create sent again returns the first endpoint")

	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, progress.Done)
	require.Equal(t, "pending", progress.State)

	progress, err = h.Progress(t.Context(), testRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, progress.Done, "the authorization policy is still updating")
	require.Equal(t, "active", progress.Waiting)

	progress, err = h.Progress(t.Context(), testRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.Nil(t, progress.Failure)

	props, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, testPolicy, props["AuthorizationPolicy"])
	require.Equal(t, id, props["Id"])
}

// policyTokens is the fake EC2 API, recording the client token of each create and each policy call.
type policyTokens struct {
	*fakeAPI
	creates  []string
	policies []string
}

func (p *policyTokens) CreateClientVpnEndpoint(ctx context.Context, in *awsec2.CreateClientVpnEndpointInput, opts ...func(*awsec2.Options)) (*awsec2.CreateClientVpnEndpointOutput, error) {
	p.creates = append(p.creates, aws.ToString(in.ClientToken))
	return p.fakeAPI.CreateClientVpnEndpoint(ctx, in, opts...)
}

func (p *policyTokens) ModifyClientVpnEndpointAuthorizationPolicy(ctx context.Context, in *awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput, opts ...func(*awsec2.Options)) (*awsec2.ModifyClientVpnEndpointAuthorizationPolicyOutput, error) {
	p.policies = append(p.policies, aws.ToString(in.ClientToken))
	return p.fakeAPI.ModifyClientVpnEndpointAuthorizationPolicy(ctx, in, opts...)
}

func Test_ClientVpnEndpoint_CreateWithPolicyToken(t *testing.T) {
	api := &policyTokens{fakeAPI: newFakeAPI(awsfake.NewCloudControl())}
	h := NewClientVpnEndpoint(api)
	request := createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": testPolicy}))

	// EC2 fails a client token reused with other parameters, so the policy call can't take the create's token.
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{request.ClientToken}, api.creates)
	require.Len(t, api.policies, 1)
	require.NotEmpty(t, api.policies[0], "the policy call sends a token")
	require.NotEqual(t, request.ClientToken, api.policies[0])

	again, err := h.Create(t.Context(), request)
	require.NoError(t, err, "a create sent again sends the same tokens")
	require.Equal(t, id, again)
	require.Equal(t, []string{request.ClientToken, request.ClientToken}, api.creates)
	require.Equal(t, []string{api.policies[0], api.policies[0]}, api.policies)
}

func Test_ClientVpnEndpoint_UpdatePolicyDefaults(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	id, err := h.Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": testPolicy})))
	require.NoError(t, err)
	current, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)

	// AWS keeps a policy value the call leaves out, so the handler sends the defaults of the values the template removed.
	document := map[string]any{"PolicyDocument": testPolicy["PolicyDocument"]}
	require.NoError(t, h.Update(t.Context(), updateRequest(t, id, current, with(current, map[string]any{"AuthorizationPolicy": document}))))
	props, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, with(document, map[string]any{"ShadowMode": "disabled"}), props["AuthorizationPolicy"])
}

// policyInputs is the fake EC2 API, recording the input of each policy call.
type policyInputs struct {
	*fakeAPI
	inputs []*awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput
}

func (p *policyInputs) ModifyClientVpnEndpointAuthorizationPolicy(ctx context.Context, in *awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput, opts ...func(*awsec2.Options)) (*awsec2.ModifyClientVpnEndpointAuthorizationPolicyOutput, error) {
	p.inputs = append(p.inputs, in)
	return p.fakeAPI.ModifyClientVpnEndpointAuthorizationPolicy(ctx, in, opts...)
}

// Test_ClientVpnEndpoint_UpdatePolicyKeepsUnmanaged checks that a policy change resets only the values the template
// removed: a value the template never set, which the patch doesn't remove, isn't sent, so AWS keeps it.
func Test_ClientVpnEndpoint_UpdatePolicyKeepsUnmanaged(t *testing.T) {
	api := &policyInputs{fakeAPI: newFakeAPI(awsfake.NewCloudControl())}
	h := NewClientVpnEndpoint(api)
	id, err := h.Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": testPolicy})))
	require.NoError(t, err)
	current, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)

	document := map[string]any{"PolicyDocument": "permit(principal, action == Action::\"connect\", resource);"}
	api.inputs = nil
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{
		Region: testRegion, Identifier: id, Current: current, Desired: with(current, map[string]any{"AuthorizationPolicy": document}),
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AuthorizationPolicy/PolicyDocument", Value: document["PolicyDocument"]}},
	}))
	require.Len(t, api.inputs, 1)
	require.Equal(t, document["PolicyDocument"], aws.ToString(api.inputs[0].PolicyDocument))
	require.Empty(t, api.inputs[0].ShadowMode, "a ShadowMode the template never set keeps AWS's value")
	require.Nil(t, api.inputs[0].Description, "a description the template never set keeps AWS's value")
}

func Test_ClientVpnEndpoint_CreateFails(t *testing.T) {
	t.Run("AWS rejects the create", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["CreateClientVpnEndpoint"] = fault("InvalidParameterValue")
		id, err := NewClientVpnEndpoint(api).Create(t.Context(), createRequest(endpointProperties()))
		require.Error(t, err)
		require.Empty(t, id)
	})
	t.Run("the policy fails after the endpoint exists", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["ModifyClientVpnEndpointAuthorizationPolicy"] = fault("InvalidParameterValue")
		id, err := NewClientVpnEndpoint(api).Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": testPolicy})))
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomePartial, failure.Outcome)
		require.NotEmpty(t, id, "UCP saves the identifier of the endpoint the create made")
	})
	t.Run("a property has the wrong type", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		_, err := NewClientVpnEndpoint(api).Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"VpnPort": "443"})))
		require.Error(t, err)
		require.Empty(t, api.calls)
	})
	t.Run("the policy has the wrong type", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		_, err := NewClientVpnEndpoint(api).Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": map[string]any{"PolicyDocument": 1}})))
		require.Error(t, err)
	})
}

func Test_ClientVpnEndpoint_Read(t *testing.T) {
	store := awsfake.NewCloudControl()
	store.Put(clientVpnEndpointType, "cvpn-endpoint-9", map[string]any{
		"ClientCidrBlock": "10.100.0.0/22",
		"AuthenticationOptions": []any{
			map[string]any{"Type": "directory-service-authentication", "ActiveDirectory": map[string]any{"DirectoryId": "d-1"}},
			map[string]any{"Type": "federated-authentication", "FederatedAuthentication": map[string]any{"SAMLProviderArn": "arn:aws:iam::123456789012:saml-provider/p"}},
		},
		"SelfServicePortal":           "enabled",
		"ClientConnectOptions":        map[string]any{"Enabled": true, "LambdaFunctionArn": "arn:aws:lambda:us-west-2:123456789012:function:f"},
		"DevicePostureOptions":        map[string]any{"Enabled": true, "TrustProviders": []any{map[string]any{"TrustProviderType": "jamf", "TenantId": "t"}}},
		"TransitGatewayConfiguration": map[string]any{"TransitGatewayId": "tgw-1", "AvailabilityZones": list("us-west-2a")},
		"AuthorizationPolicy":         map[string]any{"ShadowMode": "enabled"},
	})
	h := NewClientVpnEndpoint(newFakeAPI(store))

	props, err := h.Read(t.Context(), testRegion, "cvpn-endpoint-9")
	require.NoError(t, err)
	require.Equal(t, []any{
		map[string]any{"Type": "directory-service-authentication", "ActiveDirectory": map[string]any{"DirectoryId": "d-1"}},
		map[string]any{"Type": "federated-authentication", "FederatedAuthentication": map[string]any{"SAMLProviderArn": "arn:aws:iam::123456789012:saml-provider/p"}},
	}, props["AuthenticationOptions"])
	require.Equal(t, "enabled", props["SelfServicePortal"])
	require.Equal(t, map[string]any{"Enabled": true, "LambdaFunctionArn": "arn:aws:lambda:us-west-2:123456789012:function:f"}, props["ClientConnectOptions"])
	require.Equal(t, map[string]any{"TrustProviders": []any{map[string]any{"TrustProviderType": "jamf", "TenantId": "t"}}}, props["DevicePostureOptions"], "Enabled is write-only")
	require.Equal(t, map[string]any{"TransitGatewayId": "tgw-1", "AvailabilityZones": list("us-west-2a")}, props["TransitGatewayConfiguration"])
	require.NotContains(t, props, "AuthorizationPolicy", "a policy without a document is no policy")
	require.NotContains(t, props, "TagSpecifications", "tags AWS adds are left out")
	require.Equal(t, float64(443), props["VpnPort"])

	_, err = h.Read(t.Context(), testRegion, "cvpn-endpoint-404")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.Equal(t, "", endpointStatus(&types.ClientVpnEndpoint{}))
}

func Test_ClientVpnEndpoint_PolicyError(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	id, err := h.Create(t.Context(), createRequest(endpointProperties()))
	require.NoError(t, err)
	api.errs["GetClientVpnEndpointAuthorizationPolicy"] = fault("UnauthorizedOperation")

	_, err = h.Read(t.Context(), testRegion, id)
	require.ErrorContains(t, err, "UnauthorizedOperation", "only a missing policy reads as no policy")
	_, err = h.List(t.Context(), testRegion)
	require.ErrorContains(t, err, "UnauthorizedOperation")
}

func Test_ClientVpnEndpoint_PolicyFailed(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	id, err := h.Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"AuthorizationPolicy": testPolicy})))
	require.NoError(t, err)
	api.script[clientVpnEndpointType+"/"+id] = nil
	api.script["policy/"+id] = []string{"failed"}

	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.NotNil(t, progress.Failure)
	require.Contains(t, progress.Failure.Message, "authorization policy")
}

func Test_ClientVpnEndpoint_Update(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	id, err := h.Create(t.Context(), createRequest(with(endpointProperties(), map[string]any{"DnsServers": list("10.0.0.2"), "AuthorizationPolicy": testPolicy})))
	require.NoError(t, err)
	current, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, list("10.0.0.2"), current["DnsServers"])

	t.Run("change the DNS servers", func(t *testing.T) {
		desired := with(current, map[string]any{"DnsServers": list("10.0.0.3", "10.0.0.4")})
		require.NoError(t, h.Update(t.Context(), updateRequest(t, id, current, desired)))
		props, err := h.Read(t.Context(), testRegion, id)
		require.NoError(t, err)
		require.Equal(t, list("10.0.0.3", "10.0.0.4"), props["DnsServers"])
		current = props
	})

	t.Run("remove the DNS servers, the policy, the VPC, and the tags", func(t *testing.T) {
		api.calls = nil
		desired := with(current, map[string]any{"DnsServers": nil, "AuthorizationPolicy": nil, "VpcId": nil, "TagSpecifications": nil})
		require.NoError(t, h.Update(t.Context(), updateRequest(t, id, current, desired)))
		require.Equal(t, []string{"ModifyClientVpnEndpoint", "DeleteClientVpnEndpointAuthorizationPolicy"}, api.calls)
		props, err := h.Read(t.Context(), testRegion, id)
		require.NoError(t, err)
		require.NotContains(t, props, "DnsServers", "the endpoint uses the device's DNS servers again")
		require.NotContains(t, props, "AuthorizationPolicy")
		require.Equal(t, "vpc-1", props["VpcId"], "AWS keeps the VPC, which has no default")
	})

	t.Run("a property has the wrong type", func(t *testing.T) {
		api.calls = nil
		desired := with(current, map[string]any{"SessionTimeoutHours": "twelve"})
		require.Error(t, h.Update(t.Context(), updateRequest(t, id, current, desired)))
		require.Empty(t, api.calls)
	})

	t.Run("the endpoint is gone", func(t *testing.T) {
		desired := with(current, map[string]any{"Description": "other"})
		err := h.Update(t.Context(), updateRequest(t, "cvpn-endpoint-404", current, desired))
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_ClientVpnEndpoint_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	id, err := h.Create(t.Context(), createRequest(endpointProperties()))
	require.NoError(t, err)

	require.ErrorIs(t, h.Delete(t.Context(), testRegion, "cvpn-endpoint-404"), native.ErrNotFound)

	api.script[clientVpnEndpointType+"/"+id] = []string{"deleting", "deleting"}
	api.calls = nil
	require.NoError(t, h.Delete(t.Context(), testRegion, id), "a delete AWS already runs succeeds")
	require.NotContains(t, api.calls, "DeleteClientVpnEndpoint")

	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "deleting", Waiting: "deleted"}, progress)

	api.script[clientVpnEndpointType+"/"+id] = nil
	api.errs["DeleteClientVpnEndpoint"] = fault("InvalidClientVpnEndpointId.NotFound")
	require.ErrorIs(t, h.Delete(t.Context(), testRegion, id), native.ErrNotFound)
}

func Test_ClientVpnEndpoint_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnEndpoint(api)
	first, err := h.Create(t.Context(), createRequest(endpointProperties()))
	require.NoError(t, err)
	second, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: endpointProperties(), ClientToken: "other-token"})
	require.NoError(t, err)
	api.script[clientVpnEndpointType+"/"+second] = []string{"deleted"}

	resources, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	require.Len(t, resources, 1, "a deleted endpoint is left out")
	require.Equal(t, first, resources[0].Identifier)

	api.errs["DescribeClientVpnEndpoints"] = fault("UnauthorizedOperation")
	_, err = h.List(t.Context(), testRegion)
	require.Error(t, err)
	_, err = h.Progress(t.Context(), testRegion, first, native.OperationCreate)
	require.Error(t, err)
}
