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
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const testRegion = "us-west-2"

// updateRequest returns the update from current to desired, with the patch UCP would compute.
func updateRequest(t *testing.T, identifier string, current map[string]any, desired map[string]any) native.UpdateRequest {
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return native.UpdateRequest{Region: testRegion, Identifier: identifier, Current: current, Desired: desired, Patch: patch}
}

func createRequest(properties map[string]any) native.CreateRequest {
	return native.CreateRequest{Region: testRegion, Properties: properties, ClientToken: "11111111-2222-3333-4444-555555555555"}
}

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

func (f *fakes) failer(h native.Handler) *nativetest.Failer {
	return &f.of(h).failer
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

func list(values ...string) []any {
	out := []any{}
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

func endpointProperties() map[string]any {
	return map[string]any{
		"ClientCidrBlock":      "10.100.0.0/22",
		"ServerCertificateArn": "arn:aws:acm:us-west-2:123456789012:certificate/server",
		"AuthenticationOptions": []any{map[string]any{
			"Type":                 "certificate-authentication",
			"MutualAuthentication": map[string]any{"ClientRootCertificateChainArn": "arn:aws:acm:us-west-2:123456789012:certificate/client"},
		}},
		"ConnectionLogOptions": map[string]any{"Enabled": false},
		"Description":          "app vpn",
		"VpcId":                "vpc-1",
		"SecurityGroupIds":     list("sg-1", "sg-2"),
		"TagSpecifications":    []any{map[string]any{"ResourceType": "client-vpn-endpoint", "Tags": tags("env", "dev", "team", "a")}},
	}
}

func Test_Suite_ClientVpnEndpoint(t *testing.T) {
	var fs fakes
	endpoint := endpointProperties()
	nativetest.Run(t, nativetest.Suite{
		TypeName: clientVpnEndpointType,
		Schema:   schema(t, "aws-ec2-clientvpnendpoint.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewClientVpnEndpoint(api), api)
		},
		IdentifierProperty: "Id",
		Properties:         endpoint,
		Changed:            with(endpoint, map[string]any{"Description": "app vpn, changed", "SessionTimeoutHours": 12}),
		Removable:          "Description",
		CreateOnlyChange:   with(endpoint, map[string]any{"ClientCidrBlock": "10.200.0.0/22"}),
		OtherForm: map[string]any{
			"SecurityGroupIds":  list("sg-2", "sg-1"),
			"TagSpecifications": []any{map[string]any{"ResourceType": "client-vpn-endpoint", "Tags": tags("team", "a", "env", "dev")}},
		},
		MultiCall: with(endpoint, map[string]any{
			"SplitTunnel":         true,
			"AuthorizationPolicy": map[string]any{"PolicyDocument": "permit(principal, action, resource);", "ShadowMode": "disabled"},
		}),
		Calls:    2,
		FailCall: nativetest.FailCall(2, fs.failer),
	})
}

func Test_Suite_ClientVpnTargetNetworkAssociation(t *testing.T) {
	association := map[string]any{"ClientVpnEndpointId": "cvpn-endpoint-1", "SubnetId": "subnet-1"}
	nativetest.Run(t, nativetest.Suite{
		TypeName: clientVpnTargetNetworkAssociationType,
		Schema:   schema(t, "aws-ec2-clientvpntargetnetworkassociation.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewClientVpnTargetNetworkAssociation(newFakeAPI(store))
		},
		IdentifierProperty: "AssociationId",
		Properties:         association,
		Changed:            association,
		CreateOnlyChange:   with(association, map[string]any{"SubnetId": "subnet-2"}),
		CloudControlCreate: func(properties map[string]any) (string, map[string]any) {
			// AWS assigns the association ID, and the identifier joins it to the endpoint's.
			assigned := with(properties, map[string]any{"AssociationId": "cvpn-assoc-cc"})
			return fmt.Sprint(properties["ClientVpnEndpointId"]) + "|cvpn-assoc-cc", assigned
		},
		IdentifierProperties: func(identifier string) map[string]any {
			endpoint, association, _ := strings.Cut(identifier, "|")
			return map[string]any{"ClientVpnEndpointId": endpoint, "AssociationId": association}
		},
	})
}

func Test_Suite_NetworkAclEntry(t *testing.T) {
	entry := map[string]any{
		"NetworkAclId":  "acl-1",
		"RuleNumber":    100,
		"Protocol":      6,
		"RuleAction":    "allow",
		"Ipv6CidrBlock": "2001:DB8::/32",
		"PortRange":     map[string]any{"From": 443, "To": 443},
	}
	nativetest.Run(t, nativetest.Suite{
		TypeName: networkAclEntryType,
		Schema:   schema(t, "aws-ec2-networkaclentry.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewNetworkAclEntry(newFakeAPI(store))
		},
		IdentifierProperty: "Id",
		Properties:         entry,
		Identifier:         "acl-1|100|false",
		Changed:            with(entry, map[string]any{"RuleAction": "deny"}),
		Removable:          "PortRange",
		CreateOnlyChange:   with(entry, map[string]any{"RuleNumber": 101}),
		OtherForm:          map[string]any{"Ipv6CidrBlock": "2001:db8::/32", "RuleAction": "ALLOW", "Protocol": "6"},
		// A rule number outside 1 to 32766, including one that would wrap to 100 as an int32.
		Invalid: []map[string]any{
			with(entry, map[string]any{"RuleNumber": 0}),
			with(entry, map[string]any{"RuleNumber": 32767}),
			with(entry, map[string]any{"RuleNumber": 4294967396}),
		},
	})
}

func Test_Suite_NetworkInterfacePermission(t *testing.T) {
	permission := map[string]any{"NetworkInterfaceId": "eni-1", "AwsAccountId": "111122223333", "Permission": "INSTANCE-ATTACH"}
	nativetest.Run(t, nativetest.Suite{
		TypeName: networkInterfacePermissionType,
		Schema:   schema(t, "aws-ec2-networkinterfacepermission.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewNetworkInterfacePermission(newFakeAPI(store))
		},
		IdentifierProperty: "Id",
		Properties:         permission,
		Changed:            permission,
		CreateOnlyChange:   with(permission, map[string]any{"Permission": "EIP-ASSOCIATE"}),
		Duplicates:         true,
	})
}

// propagationTest is the route propagation handler with the fake's current entry set to the identifier each call
// works on, as the fake needs.
type propagationTest struct {
	*VPNGatewayRoutePropagation
	api *fakeAPI
}

func (p propagationTest) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p.api.setCurrent(awsapi.String(request.Properties, "VpnGatewayId") + "|" + native.GenerateName(request.ClientToken))
	return p.VPNGatewayRoutePropagation.Create(ctx, request)
}

func (p propagationTest) Update(ctx context.Context, request native.UpdateRequest) error {
	p.api.setCurrent(request.Identifier)
	return p.VPNGatewayRoutePropagation.Update(ctx, request)
}

func (p propagationTest) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	p.api.setCurrent(identifier)
	return p.VPNGatewayRoutePropagation.ReadScoped(ctx, region, identifier, managed)
}

func (p propagationTest) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	p.api.setCurrent(identifier)
	return p.VPNGatewayRoutePropagation.DeleteScoped(ctx, region, identifier, managed)
}

func propagation(gateway string, tables ...string) map[string]any {
	return map[string]any{"VpnGatewayId": gateway, "RouteTableIds": list(tables...)}
}

func Test_Suite_VPNGatewayRoutePropagation(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName: vpnGatewayRoutePropagationType,
		Schema:   schema(t, "aws-ec2-vpngatewayroutepropagation.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(propagationTest{VPNGatewayRoutePropagation: NewVPNGatewayRoutePropagation(api), api: api}, api)
		},
		IdentifierProperty: "Id",
		Properties:         propagation("vgw-1", "rtb-1", "rtb-2"),
		Changed:            propagation("vgw-1", "rtb-2", "rtb-3"),
		SharedMember:       &nativetest.SharedMember{Overlapping: propagation("vgw-1", "rtb-2", "rtb-3"), Disjoint: propagation("vgw-1", "rtb-3")},
		CreateOnlyChange:   propagation("vgw-2", "rtb-1", "rtb-2"),
		MultiCall:          propagation("vgw-1", "rtb-2", "rtb-3", "rtb-4"),
		Calls:              3,
		RejectedCalls:      1,
		FailCall:           nativetest.FailCall(3, fs.failer),
		Outside: func(store *awsfake.CloudControl, properties map[string]any) func() bool {
			gateway := awsapi.String(properties, "VpnGatewayId")
			entry := store.Resource(outsidePropagationType, gateway)
			if entry == nil {
				entry = map[string]any{"VpnGatewayId": gateway, "RouteTableIds": []any{}}
			}
			entry["RouteTableIds"] = append(entry["RouteTableIds"].([]any), "rtb-outside")
			store.Put(outsidePropagationType, gateway, entry)
			return func() bool {
				for _, typeName := range []string{vpnGatewayRoutePropagationType, outsidePropagationType} {
					for _, key := range store.List(typeName) {
						entry := store.Resource(typeName, key)
						if awsapi.String(entry, "VpnGatewayId") == gateway && slices.Contains(awsapi.Strings(entry, "RouteTableIds"), "rtb-outside") {
							return true
						}
					}
				}
				return false
			}
		},
	})
}
