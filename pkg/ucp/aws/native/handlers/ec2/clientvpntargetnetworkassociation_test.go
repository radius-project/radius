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

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// oddAssociations returns target networks AWS might return that don't match the request.
type oddAssociations struct {
	*fakeAPI
	networks []types.TargetNetwork
}

func (o oddAssociations) DescribeClientVpnTargetNetworks(ctx context.Context, in *awsec2.DescribeClientVpnTargetNetworksInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeClientVpnTargetNetworksOutput, error) {
	return &awsec2.DescribeClientVpnTargetNetworksOutput{ClientVpnTargetNetworks: o.networks}, nil
}

func (o oddAssociations) AssociateClientVpnTargetNetwork(ctx context.Context, in *awsec2.AssociateClientVpnTargetNetworkInput, _ ...func(*awsec2.Options)) (*awsec2.AssociateClientVpnTargetNetworkOutput, error) {
	return &awsec2.AssociateClientVpnTargetNetworkOutput{}, nil
}

func Test_ClientVpnTargetNetworkAssociation_Read(t *testing.T) {
	h := NewClientVpnTargetNetworkAssociation(newFakeAPI(awsfake.NewCloudControl()))

	_, err := h.Read(t.Context(), testRegion, "cvpn-endpoint-1")
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Contains(t, failure.Message, "ClientVpnEndpointId|AssociationId")

	_, err = h.Read(t.Context(), testRegion, "cvpn-endpoint-missing|cvpn-assoc-1")
	require.ErrorIs(t, err, native.ErrNotFound)

	other := types.TargetNetwork{AssociationId: aws.String("cvpn-assoc-2"), Status: &types.AssociationStatus{Code: types.AssociationStatusCodeAssociated}}
	h = NewClientVpnTargetNetworkAssociation(oddAssociations{fakeAPI: newFakeAPI(awsfake.NewCloudControl()), networks: []types.TargetNetwork{other}})
	_, err = h.Read(t.Context(), testRegion, "cvpn-endpoint-1|cvpn-assoc-1")
	require.ErrorIs(t, err, native.ErrNotFound, "another association is not this one")

	az := types.TargetNetwork{AssociationId: aws.String("cvpn-assoc-1"), ClientVpnEndpointId: aws.String("cvpn-endpoint-1"), TargetNetworkId: aws.String("tgw-attach-1")}
	h = NewClientVpnTargetNetworkAssociation(oddAssociations{fakeAPI: newFakeAPI(awsfake.NewCloudControl()), networks: []types.TargetNetwork{az}})
	props, err := h.Read(t.Context(), testRegion, "cvpn-endpoint-1|cvpn-assoc-1")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"AssociationId": "cvpn-assoc-1", "ClientVpnEndpointId": "cvpn-endpoint-1"}, props, "a target that is not a subnet has no SubnetId")
}

func Test_ClientVpnTargetNetworkAssociation_Create(t *testing.T) {
	properties := map[string]any{"ClientVpnEndpointId": "cvpn-endpoint-1", "SubnetId": "subnet-1"}
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnTargetNetworkAssociation(api)

	id, err := h.Create(t.Context(), createRequest(properties))
	require.NoError(t, err)
	require.Regexp(t, `^cvpn-endpoint-1\|cvpn-assoc-\d+$`, id)
	again, err := h.Create(t.Context(), createRequest(properties))
	require.NoError(t, err)
	require.Equal(t, id, again, "a create sent again with the client token returns the first association")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: properties, ClientToken: "other"})
	require.ErrorContains(t, err, "InvalidClientVpnDuplicateAssociationException")

	_, err = h.Create(t.Context(), createRequest(map[string]any{"ClientVpnEndpointId": "cvpn-endpoint-1", "SubnetId": 1}))
	require.Error(t, err)

	h = NewClientVpnTargetNetworkAssociation(oddAssociations{fakeAPI: newFakeAPI(awsfake.NewCloudControl())})
	_, err = h.Create(t.Context(), createRequest(properties))
	require.ErrorContains(t, err, "no association ID")
}

func Test_ClientVpnTargetNetworkAssociation_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnTargetNetworkAssociation(api)
	id, err := h.Create(t.Context(), createRequest(map[string]any{"ClientVpnEndpointId": "cvpn-endpoint-1", "SubnetId": "subnet-1"}))
	require.NoError(t, err)

	require.ErrorIs(t, h.Delete(t.Context(), testRegion, "cvpn-endpoint-1|cvpn-assoc-404"), native.ErrNotFound)

	api.script[clientVpnTargetNetworkAssociationType+"/"+id] = []string{"disassociating"}
	api.calls = nil
	require.NoError(t, h.Delete(t.Context(), testRegion, id), "a delete AWS already runs succeeds")
	require.NotContains(t, api.calls, "DisassociateClientVpnTargetNetwork")

	require.NoError(t, h.Delete(t.Context(), testRegion, id))
	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "disassociating", Waiting: "disassociated"}, progress)
	_, err = h.Progress(t.Context(), testRegion, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "a disassociated association is gone")
}

func Test_ClientVpnTargetNetworkAssociation_NoUpdateNoList(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewClientVpnTargetNetworkAssociation(api)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: "cvpn-endpoint-1|cvpn-assoc-1"}))
	require.Empty(t, api.calls)
	_, err := h.List(t.Context(), testRegion)
	require.ErrorContains(t, err, "can't be listed")
}
