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

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func permissionProps() map[string]any {
	return map[string]any{"NetworkInterfaceId": "eni-1", "AwsAccountId": "111122223333", "Permission": "INSTANCE-ATTACH"}
}

// emptyPermission returns a create response without the permission.
type emptyPermission struct {
	*fakeAPI
}

func (e emptyPermission) CreateNetworkInterfacePermission(ctx context.Context, in *awsec2.CreateNetworkInterfacePermissionInput, _ ...func(*awsec2.Options)) (*awsec2.CreateNetworkInterfacePermissionOutput, error) {
	return &awsec2.CreateNetworkInterfacePermissionOutput{}, nil
}

func Test_NetworkInterfacePermission_Create(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkInterfacePermission(api)
	id, err := h.Create(t.Context(), createRequest(permissionProps()))
	require.NoError(t, err)
	require.Regexp(t, `^eni-perm-\d+$`, id)

	api.errs["CreateNetworkInterfacePermission"] = fault("InvalidNetworkInterfaceID.NotFound")
	_, err = h.Create(t.Context(), createRequest(permissionProps()))
	require.Error(t, err)

	_, err = NewNetworkInterfacePermission(emptyPermission{newFakeAPI(awsfake.NewCloudControl())}).Create(t.Context(), createRequest(permissionProps()))
	require.ErrorContains(t, err, "no permission ID")
}

func Test_NetworkInterfacePermission_Find(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkInterfacePermission(api)
	request := createRequest(permissionProps())

	found, err := h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, found, "AWS has none")

	first, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	_, err = h.Create(t.Context(), createRequest(with(permissionProps(), map[string]any{"Permission": "EIP-ASSOCIATE"})))
	require.NoError(t, err)
	_, err = h.Create(t.Context(), createRequest(with(permissionProps(), map[string]any{"AwsAccountId": "444455556666"})))
	require.NoError(t, err)
	found, err = h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, first, found, "another permission or account is not a match")

	second, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	_, err = h.Find(t.Context(), request)
	require.ErrorContains(t, err, "can't tell", "with two matches, UCP can't tell which one it created")

	require.NoError(t, h.Delete(t.Context(), testRegion, second))
	for range 2 {
		_, err = h.Progress(t.Context(), testRegion, second, native.OperationDelete)
	}
	require.ErrorIs(t, err, native.ErrNotFound)
	found, err = h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, first, found, "a revoked permission is left out")

	api.errs["DescribeNetworkInterfacePermissions"] = fault("UnauthorizedOperation")
	_, err = h.Find(t.Context(), request)
	require.Error(t, err)
}

func Test_NetworkInterfacePermission_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkInterfacePermission(api)
	id, err := h.Create(t.Context(), createRequest(permissionProps()))
	require.NoError(t, err)

	require.ErrorIs(t, h.Delete(t.Context(), testRegion, "eni-perm-404"), native.ErrNotFound)

	api.script[networkInterfacePermissionType+"/"+id] = []string{"revoking"}
	api.calls = nil
	require.NoError(t, h.Delete(t.Context(), testRegion, id), "a delete AWS already runs succeeds")
	require.NotContains(t, api.calls, "DeleteNetworkInterfacePermission")

	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id}), "every property is create-only")
}

func Test_NetworkInterfacePermission_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewNetworkInterfacePermission(api)
	kept, err := h.Create(t.Context(), createRequest(permissionProps()))
	require.NoError(t, err)
	revoked, err := h.Create(t.Context(), createRequest(with(permissionProps(), map[string]any{"Permission": "EIP-ASSOCIATE"})))
	require.NoError(t, err)
	api.script[networkInterfacePermissionType+"/"+revoked] = []string{"revoked"}

	resources, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	require.Len(t, resources, 1, "a revoked permission is left out")
	require.Equal(t, kept, resources[0].Identifier)
	require.Equal(t, map[string]any{"Id": kept, "NetworkInterfaceId": "eni-1", "AwsAccountId": "111122223333", "Permission": "INSTANCE-ATTACH"}, resources[0].Properties)

	api.errs["DescribeNetworkInterfacePermissions"] = fault("UnauthorizedOperation")
	_, err = h.List(t.Context(), testRegion)
	require.Error(t, err)
}
