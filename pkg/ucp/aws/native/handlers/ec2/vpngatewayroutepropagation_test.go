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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func newPropagationTest() (propagationTest, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return propagationTest{VPNGatewayRoutePropagation: NewVPNGatewayRoutePropagation(api), api: api}, api
}

func Test_VPNGatewayRoutePropagation_Create(t *testing.T) {
	h, api := newPropagationTest()
	request := createRequest(propagation("vgw-1", "rtb-1", "rtb-2", "rtb-1"))
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "vgw-1|"+native.GenerateName(request.ClientToken), id)
	require.Equal(t, []string{"EnableVgwRoutePropagation", "EnableVgwRoutePropagation"}, api.calls, "one call per route table, without repeats")

	again, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, id, again, "a create sent again changes nothing and returns the same identifier")

	_, err = h.Create(t.Context(), createRequest(map[string]any{"VpnGatewayId": "vgw-1"}))
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)

	h, _ = newPropagationTest()
	id, err = h.Create(t.Context(), createRequest(propagation("missing", "rtb-1")))
	require.ErrorContains(t, err, "InvalidVpnGatewayID.NotFound")
	require.Empty(t, id, "AWS rejected the first call, so nothing exists")

	h, _ = newPropagationTest()
	id, err = h.Create(t.Context(), createRequest(propagation("vgw-1", "rtb-1", "missing")))
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomePartial, failure.Outcome)
	require.NotEmpty(t, id, "UCP saves the identifier of a create that changed AWS")
}

func Test_VPNGatewayRoutePropagation_Read(t *testing.T) {
	h, api := newPropagationTest()
	id, err := h.Create(t.Context(), createRequest(propagation("vgw-1", "rtb-2", "rtb-1")))
	require.NoError(t, err)
	api.store.Put(outsidePropagationType, "vgw-1", propagation("vgw-1", "rtb-outside"))
	api.store.Put(outsidePropagationType, "vgw-2", propagation("vgw-2", "rtb-3"))

	props, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Id": id, "VpnGatewayId": "vgw-1", "RouteTableIds": list("rtb-1", "rtb-2", "rtb-outside")}, props, "with no record, every route table the gateway propagates to")

	props, err = h.ReadScoped(t.Context(), testRegion, id, []map[string]any{propagation("vgw-1", "rtb-2"), propagation("vgw-1", "rtb-1", "rtb-gone")})
	require.NoError(t, err)
	require.Equal(t, list("rtb-1", "rtb-2"), props["RouteTableIds"], "only the route tables the alias manages")

	props, err = h.ReadScoped(t.Context(), testRegion, "id-1", []map[string]any{propagation("vgw-2", "rtb-3")})
	require.NoError(t, err)
	require.Equal(t, "vgw-2", props["VpnGatewayId"], "an identifier without the gateway takes it from the properties")

	_, err = h.ReadScoped(t.Context(), testRegion, id, []map[string]any{{"VpnGatewayId": "vgw-1"}})
	require.ErrorIs(t, err, native.ErrNotFound, "the alias manages no route table")
	_, err = h.ReadScoped(t.Context(), testRegion, id, []map[string]any{propagation("vgw-1", "rtb-3")})
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Read(t.Context(), testRegion, "id-1")
	var failure *native.Failure
	require.ErrorAs(t, err, &failure, "an identifier without the gateway, and no properties")
	require.Contains(t, failure.Message, "VpnGatewayId|name")

	api.errs["DescribeRouteTables"] = fault("UnauthorizedOperation")
	_, err = h.Read(t.Context(), testRegion, id)
	require.Error(t, err)
}

func Test_VPNGatewayRoutePropagation_Update(t *testing.T) {
	h, api := newPropagationTest()
	id, err := h.Create(t.Context(), createRequest(propagation("vgw-1", "rtb-1", "rtb-2")))
	require.NoError(t, err)

	api.calls = nil
	require.NoError(t, h.Update(t.Context(), updateRequest(t, id, propagation("vgw-1", "rtb-1", "rtb-2"), propagation("vgw-1", "rtb-2", "rtb-3"))))
	require.Equal(t, []string{"DisableVgwRoutePropagation", "EnableVgwRoutePropagation"}, api.calls, "removals first")

	api.errs["DisableVgwRoutePropagation"] = fault("UnauthorizedOperation")
	err = h.Update(t.Context(), updateRequest(t, id, propagation("vgw-1", "rtb-2", "rtb-3"), propagation("vgw-1", "rtb-3")))
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, "a failed removal keeps every route table on the record")
	require.True(t, failure.ClientError)

	require.Error(t, h.Update(t.Context(), updateRequest(t, "id-1", map[string]any{}, map[string]any{})))
}

func Test_VPNGatewayRoutePropagation_DeleteScoped(t *testing.T) {
	h, api := newPropagationTest()
	id, err := h.Create(t.Context(), createRequest(propagation("vgw-1", "rtb-1", "rtb-2")))
	require.NoError(t, err)
	managed := []map[string]any{propagation("vgw-1", "rtb-1", "rtb-2")}

	require.ErrorIs(t, h.DeleteScoped(t.Context(), testRegion, id, nil), native.ErrNotFound, "with no record, nothing is turned off")
	require.ErrorContains(t, h.DeleteScoped(t.Context(), testRegion, "id-1", nil), "VpnGatewayId|name")

	api.errs["DisableVgwRoutePropagation"] = fault("UnauthorizedOperation")
	require.Error(t, h.DeleteScoped(t.Context(), testRegion, id, managed))

	api.errs["DisableVgwRoutePropagation"] = fault("InvalidRouteTableID.NotFound")
	require.NoError(t, h.DeleteScoped(t.Context(), testRegion, id, managed), "a route table that is gone has no propagation to turn off")

	delete(api.errs, "DisableVgwRoutePropagation")
	require.NoError(t, h.DeleteScoped(t.Context(), testRegion, id, managed))
	require.ErrorIs(t, h.DeleteScoped(t.Context(), testRegion, id, managed), native.ErrNotFound)
}

func Test_VPNGatewayRoutePropagation_Unsupported(t *testing.T) {
	h, api := newPropagationTest()
	require.ErrorContains(t, h.Delete(t.Context(), testRegion, "vgw-1|radius-0123456789ab"), "DeleteScoped")
	_, err := h.List(t.Context(), testRegion)
	require.ErrorContains(t, err, "can't be listed")
	progress, err := h.Progress(t.Context(), testRegion, "vgw-1|radius-0123456789ab", native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.Empty(t, api.calls)
}
