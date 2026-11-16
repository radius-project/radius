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
	"time"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// recordedEndpoint returns the template of the smallest Client VPN endpoint: certificate authentication, no
// connection logs, and no target network. vpc is the VPC to use, or "" for the default VPC.
func recordedEndpoint(s *awsreplay.Session, vpc string) map[string]any {
	run := s.Input("RADIUS_REC_RUN")
	properties := map[string]any{
		"ClientCidrBlock":      "10.250.0.0/22",
		"ServerCertificateArn": s.Input("RADIUS_REC_CLIENT_VPN_SERVER_CERTIFICATE"),
		"AuthenticationOptions": []any{map[string]any{
			"Type":                 "certificate-authentication",
			"MutualAuthentication": map[string]any{"ClientRootCertificateChainArn": s.Input("RADIUS_REC_CLIENT_VPN_CLIENT_CERTIFICATE")},
		}},
		"ConnectionLogOptions": map[string]any{"Enabled": false},
		"Description":          "radius-rec-" + run,
		"TagSpecifications":    []any{map[string]any{"ResourceType": "client-vpn-endpoint", "Tags": tags("radius-recording", run)}},
	}
	if vpc != "" {
		properties["VpcId"] = vpc
	}
	return properties
}

// Test_Replay_ClientVpnEndpoint creates a Client VPN endpoint, changes its description and session timeout, and
// deletes it, against AWS's recorded responses.
func Test_Replay_ClientVpnEndpoint(t *testing.T) {
	s := awsreplay.New(t, "testdata/clientvpnendpoint.json")
	h := NewClientVpnEndpoint(awsec2.NewFromConfig(s.Config(t.Context())))
	properties := recordedEndpoint(s, "")
	updated := with(properties, map[string]any{"Description": "radius-rec-" + s.Input("RADIUS_REC_RUN") + "-updated", "SessionTimeoutHours": 12})
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: properties,
		// AWS may return ConnectionLogOptions with more fields than the template sets.
		Want:        with(properties, map[string]any{"ConnectionLogOptions": nil}),
		Update:      updated,
		WantUpdated: with(updated, map[string]any{"ConnectionLogOptions": nil}),
		Poll:        10 * time.Second,
	}.Run(t)
}

// createdEndpoint is the endpoint handler that keeps the identifier its create returned.
type createdEndpoint struct {
	*ClientVpnEndpoint
	identifier *string
}

func (c createdEndpoint) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := c.ClientVpnEndpoint.Create(ctx, request)
	*c.identifier = identifier
	return identifier, err
}

// Test_Replay_ClientVpnTargetNetworkAssociation creates a Client VPN endpoint in the recording VPC, associates a subnet
// with it, disassociates it, and deletes the endpoint, against AWS's recorded responses.
func Test_Replay_ClientVpnTargetNetworkAssociation(t *testing.T) {
	s := awsreplay.New(t, "testdata/clientvpntargetnetworkassociation.json")
	api := awsec2.NewFromConfig(s.Config(t.Context()))
	endpointID := ""
	endpointProperties := recordedEndpoint(s, s.Input("RADIUS_REC_VPC"))
	association := nativetest.Lifecycle{
		Handler: NewClientVpnTargetNetworkAssociation(api),
		Session: s,
		Poll:    30 * time.Second,
	}

	// The association needs the endpoint, so the endpoint's lifecycle runs around it.
	nativetest.Lifecycle{
		Handler:    createdEndpoint{ClientVpnEndpoint: NewClientVpnEndpoint(api), identifier: &endpointID},
		Session:    s,
		Properties: endpointProperties,
		Want:       with(endpointProperties, map[string]any{"ConnectionLogOptions": nil}),
		Poll:       10 * time.Second,
		Between: func() {
			association.Properties = map[string]any{"ClientVpnEndpointId": endpointID, "SubnetId": s.Input("RADIUS_REC_SUBNET")}
			association.Run(t)
		},
	}.Run(t)
}

// Test_Replay_NetworkAclEntry adds an entry to the recording network ACL, changes its action, and removes it, against
// AWS's recorded responses.
func Test_Replay_NetworkAclEntry(t *testing.T) {
	s := awsreplay.New(t, "testdata/networkaclentry.json")
	h := NewNetworkAclEntry(awsec2.NewFromConfig(s.Config(t.Context())))
	properties := map[string]any{
		"NetworkAclId": s.Input("RADIUS_REC_NETWORK_ACL"),
		"RuleNumber":   100,
		"Protocol":     6,
		"RuleAction":   "allow",
		"CidrBlock":    "10.0.0.0/16",
		"PortRange":    map[string]any{"From": 443, "To": 443},
	}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: properties,
		Update:     with(properties, map[string]any{"RuleAction": "deny"}),
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_NetworkInterfacePermission grants another account permission to attach the recording network interface,
// checks that UCP finds the permission by its values, and revokes it, against responses written from the AWS API
// reference. AWS rejects the create for an account that is not allowed to grant cross-account permissions, so the
// cassette is not recorded. Whether AWS rejects a second permission with the same values is not verified.
func Test_Replay_NetworkInterfacePermission(t *testing.T) {
	s := awsreplay.New(t, "testdata/networkinterfacepermission.json")
	h := NewNetworkInterfacePermission(awsec2.NewFromConfig(s.Config(t.Context())))
	properties := map[string]any{
		"NetworkInterfaceId": s.Input("RADIUS_REC_NETWORK_INTERFACE"),
		"AwsAccountId":       s.Input("RADIUS_REC_OTHER_ACCOUNT"),
		"Permission":         "INSTANCE-ATTACH",
	}
	request := native.CreateRequest{Region: awsreplay.Region, Properties: properties, ClientToken: "33333333-4444-5555-6666-777777777777"}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: properties,
		Poll:       5 * time.Second,
		Between: func() {
			found, err := h.Find(t.Context(), request)
			require.NoError(t, err)
			require.NotEmpty(t, found, "UCP finds the permission by the network interface, account, and permission")
		},
	}.Run(t)
}

// Test_Replay_VPNGatewayRoutePropagation turns on route propagation from the recording virtual private gateway to one
// route table, moves it to another, and turns it off, against AWS's recorded responses.
func Test_Replay_VPNGatewayRoutePropagation(t *testing.T) {
	s := awsreplay.New(t, "testdata/vpngatewayroutepropagation.json")
	h := NewVPNGatewayRoutePropagation(awsec2.NewFromConfig(s.Config(t.Context())))
	gateway := s.Input("RADIUS_REC_VPN_GATEWAY")
	properties := propagation(gateway, s.Input("RADIUS_REC_ROUTE_TABLE"))
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: properties,
		Update:     propagation(gateway, s.Input("RADIUS_REC_ROUTE_TABLE_2")),
		Poll:       5 * time.Second,
	}.Run(t)
}
