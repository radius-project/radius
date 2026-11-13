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
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference      = "https://docs.aws.amazon.com/AWSEC2/latest/APIReference/"
	commonErrors      = apiReference + "errors-overview.html"
	pagination        = apiReference + "Query-Requests.html#api-pagination"
	idempotency       = "https://docs.aws.amazon.com/ec2/latest/devguide/ec2-api-idempotency.html"
	naclRules         = "https://docs.aws.amazon.com/vpc/latest/userguide/nacl-rules.html"
	defaultNacl       = "https://docs.aws.amazon.com/vpc/latest/userguide/default-network-acl.html"
	naclEntryTemplate = "https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ec2-networkaclentry.html"

	// codes says which error codes the fake returns for the rules of a page that names none.
	codes = "The page names no error codes. The fake returns the codes of EC2's error codes page: MissingParameter for " +
		"a parameter left out, InvalidParameterCombination for parameters that can't be combined, and " +
		"InvalidParameterValue for any other value EC2 doesn't take."
)

// page returns the page of the call or data type in EC2's API reference.
func page(name string) string {
	return apiReference + "API_" + name + ".html"
}

// inTestRegion is the option that sends a call to the test Region.
var inTestRegion = inRegion(testRegion)

// breaks returns a Break that makes the call against a new fake of the EC2 API backed by store.
func breaks(call func(ctx context.Context, f *fakeAPI) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return call(ctx, newFakeAPI(store))
	}
}

// endpointInput returns a request for an endpoint EC2 takes, after change makes its changes, when it isn't nil.
func endpointInput(change func(in *awsec2.CreateClientVpnEndpointInput)) *awsec2.CreateClientVpnEndpointInput {
	in := &awsec2.CreateClientVpnEndpointInput{
		ClientCidrBlock:      aws.String("10.100.0.0/22"),
		ServerCertificateArn: aws.String("arn:aws:acm:us-west-2:123456789012:certificate/server"),
		AuthenticationOptions: []types.ClientVpnAuthenticationRequest{{
			Type:                 types.ClientVpnAuthenticationTypeCertificateAuthentication,
			MutualAuthentication: &types.CertificateAuthenticationRequest{ClientRootCertificateChainArn: aws.String("arn:aws:acm:us-west-2:123456789012:certificate/client")},
		}},
		ConnectionLogOptions: &types.ConnectionLogOptions{Enabled: aws.Bool(false)},
		ClientToken:          aws.String("token-1"),
	}
	if change != nil {
		change(in)
	}
	return in
}

// createEndpoint creates an endpoint from endpointInput and returns its ID.
func createEndpoint(ctx context.Context, f *fakeAPI, change func(in *awsec2.CreateClientVpnEndpointInput)) (string, error) {
	out, err := f.CreateClientVpnEndpoint(ctx, endpointInput(change), inTestRegion)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.ClientVpnEndpointId), nil
}

// rejectEndpoint returns a Break that creates an endpoint with the change.
func rejectEndpoint(change func(in *awsec2.CreateClientVpnEndpointInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return breaks(func(ctx context.Context, f *fakeAPI) error {
		_, err := createEndpoint(ctx, f, change)
		return err
	})
}

// describeEndpoint returns the endpoint with the ID.
func describeEndpoint(ctx context.Context, f *fakeAPI, id string) (types.ClientVpnEndpoint, error) {
	out, err := f.DescribeClientVpnEndpoints(ctx, &awsec2.DescribeClientVpnEndpointsInput{ClientVpnEndpointIds: []string{id}}, inTestRegion)
	if err != nil {
		return types.ClientVpnEndpoint{}, err
	}
	return out.ClientVpnEndpoints[0], nil
}

// endpointStatusIs returns an error when the endpoint's status isn't want.
func endpointStatusIs(ctx context.Context, f *fakeAPI, id string, want types.ClientVpnEndpointStatusCode) error {
	e, err := describeEndpoint(ctx, f, id)
	if err != nil {
		return err
	}
	if e.Status.Code != want {
		return fmt.Errorf("the endpoint is %s, not %s", e.Status.Code, want)
	}
	return nil
}

// newEndpoint creates an endpoint, and reads it once, so its status is the one it keeps.
func newEndpoint(ctx context.Context, f *fakeAPI) (string, error) {
	id, err := createEndpoint(ctx, f, nil)
	if err != nil {
		return "", err
	}
	_, err = describeEndpoint(ctx, f, id)
	return id, err
}

// modifyEndpoint returns a Break that creates an endpoint, then modifies it as change says.
func modifyEndpoint(change func(in *awsec2.ModifyClientVpnEndpointInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return breaks(func(ctx context.Context, f *fakeAPI) error {
		id, err := createEndpoint(ctx, f, nil)
		if err != nil {
			return err
		}
		in := &awsec2.ModifyClientVpnEndpointInput{ClientVpnEndpointId: aws.String(id)}
		change(in)
		_, err = f.ModifyClientVpnEndpoint(ctx, in, inTestRegion)
		return err
	})
}

// setPolicy sets the authorization policy of the endpoint with the ID.
func setPolicy(ctx context.Context, f *fakeAPI, id string, in *awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput) error {
	in.ClientVpnEndpointId = aws.String(id)
	_, err := f.ModifyClientVpnEndpointAuthorizationPolicy(ctx, in, inTestRegion)
	return err
}

// getPolicy returns the authorization policy of the endpoint with the ID.
func getPolicy(ctx context.Context, f *fakeAPI, id string) (*awsec2.GetClientVpnEndpointAuthorizationPolicyOutput, error) {
	return f.GetClientVpnEndpointAuthorizationPolicy(ctx, &awsec2.GetClientVpnEndpointAuthorizationPolicyInput{ClientVpnEndpointId: aws.String(id)}, inTestRegion)
}

// clientVpnEndpointAPI records the EC2 calls ClientVpnEndpoint makes.
var clientVpnEndpointAPI = map[string]nativetest.Call{
	"CreateClientVpnEndpoint": {
		Doc: page("CreateClientVpnEndpoint"),
		Rules: []nativetest.Rule{
			{
				Rule:  "Authentication, ConnectionLogOptions, and ServerCertificateArn are required",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.ServerCertificateArn = nil }),
				Code:  missingParameter,
			},
			{
				Rule: "ClientCidrBlock is an IPv4 block from /12 to /22",
				Doc:  commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.ClientCidrBlock = aws.String("10.100.0.0/23")
				}),
				Code: invalidValue,
			},
			{
				Rule: "an authentication option needs the settings of its Type, such as MutualAuthentication for certificate-authentication",
				Doc:  page("ClientVpnAuthenticationRequest"),
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.AuthenticationOptions[0].MutualAuthentication = nil
				}),
				Code: missingParameter,
			},
			{
				Rule: "connection logging that is on needs CloudwatchLogGroup",
				Doc:  page("ConnectionLogOptions"),
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.ConnectionLogOptions = &types.ConnectionLogOptions{Enabled: aws.Bool(true)}
				}),
				Code: missingParameter,
			},
			{
				Rule: "an endpoint has up to two DNS servers",
				Doc:  commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.DnsServers = []string{"10.0.0.2", "10.0.0.3", "10.0.0.4"}
				}),
				Code: invalidValue,
			},
			{
				Rule: "SecurityGroupIds need VpcId, the VPC of the security groups",
				Doc:  commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.SecurityGroupIds = []string{"sg-1"}
				}),
				Code: missingParameter,
			},
			{
				Rule: "TransitGatewayConfiguration can't be sent with VpcId or SecurityGroupIds",
				Doc:  commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.VpcId = aws.String("vpc-1")
					in.TransitGatewayConfiguration = &types.TransitGatewayConfigurationInputStructure{TransitGatewayId: aws.String("tgw-1")}
				}),
				Code: invalidCombination,
			},
			{
				Rule: "TransitGatewayConfiguration can't have both AvailabilityZones and AvailabilityZoneIds",
				Doc:  page("TransitGatewayConfigurationInputStructure"),
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.TransitGatewayConfiguration = &types.TransitGatewayConfigurationInputStructure{
						TransitGatewayId:    aws.String("tgw-1"),
						AvailabilityZones:   []string{"us-west-2a"},
						AvailabilityZoneIds: []string{"usw2-az1"},
					}
				}),
				Code: invalidCombination,
			},
			{
				Rule:  "SessionTimeoutHours is 8, 10, 12, or 24",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.SessionTimeoutHours = aws.Int32(9) }),
				Code:  invalidValue,
			},
			{
				Rule:  "VpnPort is 443 or 1194",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.VpnPort = aws.Int32(80) }),
				Code:  invalidValue,
			},
			{
				Rule:  "a VpnPort of 0 isn't 443 or 1194",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.VpnPort = aws.Int32(0) }),
				Code:  invalidValue,
			},
			{
				Rule:  "a SessionTimeoutHours of 0 isn't 8, 10, 12, or 24",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.SessionTimeoutHours = aws.Int32(0) }),
				Code:  invalidValue,
			},
			{
				Rule:  "an empty ClientCidrBlock isn't a block from /12 to /22",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.ClientCidrBlock = aws.String("") }),
				Code:  invalidValue,
			},
			{
				Rule: "TransitGatewayConfiguration can't be sent with VpcId, even an empty one",
				Doc:  commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.VpcId = aws.String("")
					in.TransitGatewayConfiguration = &types.TransitGatewayConfigurationInputStructure{TransitGatewayId: aws.String("tgw-1")}
				}),
				Code: invalidCombination,
			},
			{
				Rule:  "TransportProtocol is tcp or udp",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.TransportProtocol = "sctp" }),
				Code:  invalidValue,
			},
			{
				Rule:  "SelfServicePortal is enabled or disabled",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.SelfServicePortal = "on" }),
				Code:  invalidValue,
			},
			{
				Rule:  "EndpointIpAddressType and TrafficIpAddressType are ipv4, ipv6, or dual-stack",
				Doc:   commonErrors,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) { in.TrafficIpAddressType = "ipv5" }),
				Code:  invalidValue,
			},
			{
				Rule: "the login banner's BannerText is up to 1400 characters",
				Doc:  page("ClientLoginBannerOptions"),
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.ClientLoginBannerOptions = &types.ClientLoginBannerOptions{Enabled: aws.Bool(true), BannerText: aws.String(strings.Repeat("a", 1401))}
				}),
				Code: invalidValue,
			},
			{
				Rule: "the call tags only the client-vpn-endpoint resource type",
				Doc:  page("TagSpecification"),
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.TagSpecifications = []types.TagSpecification{{ResourceType: types.ResourceTypeVpc, Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("dev")}}}}
				}),
				Code: invalidValue,
			},
			{
				Rule: "VpnPort defaults to 443, SessionTimeoutHours to 24, TransportProtocol to udp, SplitTunnel to false, " +
					"DisconnectOnSessionTimeout to true, and EndpointIpAddressType and TrafficIpAddressType to ipv4",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					e, err := describeEndpoint(ctx, f, id)
					if err != nil {
						return err
					}
					got := []any{aws.ToInt32(e.VpnPort), aws.ToInt32(e.SessionTimeoutHours), e.TransportProtocol, aws.ToBool(e.SplitTunnel), aws.ToBool(e.DisconnectOnSessionTimeout), e.EndpointIpAddressType, e.TrafficIpAddressType}
					want := []any{int32(443), int32(24), types.TransportProtocolUdp, false, true, types.EndpointIpAddressTypeIpv4, types.TrafficIpAddressTypeIpv4}
					if !slices.Equal(got, want) {
						return fmt.Errorf("the endpoint has %v, not the defaults %v", got, want)
					}
					return nil
				},
			},
			{
				Rule: "an endpoint with VpcId and no SecurityGroupIds gets the VPC's default security group",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := createEndpoint(ctx, f, func(in *awsec2.CreateClientVpnEndpointInput) { in.VpcId = aws.String("vpc-1") })
					if err != nil {
						return err
					}
					e, err := describeEndpoint(ctx, f, id)
					if err != nil {
						return err
					}
					if !slices.Equal(e.SecurityGroupIds, []string{defaultSecurityGroup("vpc-1")}) {
						return fmt.Errorf("the endpoint has the security groups %v", e.SecurityGroupIds)
					}
					return nil
				},
			},
			{
				Rule: "a client token is up to 64 ASCII characters",
				Doc:  idempotency,
				Break: rejectEndpoint(func(in *awsec2.CreateClientVpnEndpointInput) {
					in.ClientToken = aws.String(strings.Repeat("t", 65))
				}),
				Code: "InvalidClientToken",
			},
			{
				Rule: "a request sent again with the same client token and parameters returns the first endpoint",
				Doc:  idempotency,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					first, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					again, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					if again != first {
						return fmt.Errorf("the request sent again made the endpoint %s, not %s", again, first)
					}
					return nil
				},
			},
			{
				Rule: "a client token sent again with other parameters fails",
				Doc:  idempotency,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					if _, err := createEndpoint(ctx, f, nil); err != nil {
						return err
					}
					_, err := createEndpoint(ctx, f, func(in *awsec2.CreateClientVpnEndpointInput) { in.Description = aws.String("other") })
					return err
				}),
				Code: "IdempotentParameterMismatch",
			},
			{
				Rule: "a client token that another call's request had fails",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					if _, err := associate(ctx, f, nil); err != nil {
						return err
					}
					_, err := createEndpoint(ctx, f, func(in *awsec2.CreateClientVpnEndpointInput) { in.ClientToken = aws.String("association-token") })
					return err
				}),
				Code: "IdempotentParameterMismatch",
			},
			{
				Rule: "a client token is unique in a Region, so the same request in another Region makes another endpoint",
				Doc:  idempotency,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					first, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					out, err := f.CreateClientVpnEndpoint(ctx, endpointInput(nil), inRegion("us-east-1"))
					if err != nil {
						return err
					}
					if aws.ToString(out.ClientVpnEndpointId) == first {
						return fmt.Errorf("the request in another Region returned the endpoint %s", first)
					}
					return nil
				},
			},
		},
		Unclear: codes + " The page says SelfServicePortal defaults to enabled, and the Client VPN guide says the portal " +
			"isn't available with mutual authentication and must be turned on; the fake turns it on only when asked. " +
			"Neither page says how long EC2 keeps a client token, or what a request sent again does after its endpoint " +
			"is deleted; the fake makes a new endpoint. The fake doesn't check that ClientCidrBlock doesn't overlap the " +
			"VPC, and returns InvalidParameterValue for a tag specification without ResourceType, a code the pages " +
			"don't give.",
	},
	"DescribeClientVpnEndpoints": {
		Doc: page("DescribeClientVpnEndpoints"),
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 5 to 1000",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeClientVpnEndpoints(ctx, &awsec2.DescribeClientVpnEndpointsInput{MaxResults: aws.Int32(4)}, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "MaxResults can't be sent with endpoint IDs",
				Doc:  pagination,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					_, err = f.DescribeClientVpnEndpoints(ctx, &awsec2.DescribeClientVpnEndpointsInput{ClientVpnEndpointIds: []string{id}, MaxResults: aws.Int32(5)}, inTestRegion)
					return err
				}),
				Code: invalidCombination,
			},
			{
				Rule: "an endpoint ID that doesn't exist fails",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := describeEndpoint(ctx, f, missingEndpoint)
					return err
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
			{
				Rule: "an endpoint is pending-associate until a target network is associated with it, then available",
				Doc:  page("ClientVpnEndpointStatus"),
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := newEndpoint(ctx, f)
					if err != nil {
						return err
					}
					if err := endpointStatusIs(ctx, f, id, types.ClientVpnEndpointStatusCodePendingAssociate); err != nil {
						return err
					}
					if _, err := associate(ctx, f, func(in *awsec2.AssociateClientVpnTargetNetworkInput) { in.ClientVpnEndpointId = aws.String(id) }); err != nil {
						return err
					}
					return endpointStatusIs(ctx, f, id, types.ClientVpnEndpointStatusCodeAvailable)
				},
			},
		},
		Unclear: "The status page says an endpoint is pending only with a transit gateway; the fake reports pending " +
			"once after any create. Neither page says how long EC2 lists a deleted endpoint; the fake lists it once.",
	},
	"ModifyClientVpnEndpoint": {
		Doc: page("ModifyClientVpnEndpoint"),
		Rules: []nativetest.Rule{
			{
				Rule: "the endpoint exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ModifyClientVpnEndpoint(ctx, &awsec2.ModifyClientVpnEndpointInput{ClientVpnEndpointId: aws.String(missingEndpoint), Description: aws.String("d")}, inTestRegion)
					return err
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
			{
				Rule:  "SessionTimeoutHours is 8, 10, 12, or 24",
				Doc:   commonErrors,
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) { in.SessionTimeoutHours = aws.Int32(48) }),
				Code:  invalidValue,
			},
			{
				Rule:  "VpnPort is 443 or 1194",
				Doc:   commonErrors,
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) { in.VpnPort = aws.Int32(8443) }),
				Code:  invalidValue,
			},
			{
				Rule:  "a VpnPort of 0 isn't 443 or 1194",
				Doc:   commonErrors,
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) { in.VpnPort = aws.Int32(0) }),
				Code:  invalidValue,
			},
			{
				Rule:  "a SessionTimeoutHours of 0 isn't 8, 10, 12, or 24",
				Doc:   commonErrors,
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) { in.SessionTimeoutHours = aws.Int32(0) }),
				Code:  invalidValue,
			},
			{
				Rule:  "SelfServicePortal is enabled or disabled",
				Doc:   commonErrors,
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) { in.SelfServicePortal = "true" }),
				Code:  invalidValue,
			},
			{
				Rule: "an endpoint has up to two DNS servers",
				Doc:  page("DnsServersOptionsModifyStructure"),
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) {
					in.DnsServers = &types.DnsServersOptionsModifyStructure{Enabled: aws.Bool(true), CustomDnsServers: []string{"10.0.0.2", "10.0.0.3", "10.0.0.4"}}
				}),
				Code: invalidValue,
			},
			{
				Rule: "connection logging that is on needs CloudwatchLogGroup",
				Doc:  page("ConnectionLogOptions"),
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) {
					in.ConnectionLogOptions = &types.ConnectionLogOptions{Enabled: aws.Bool(true)}
				}),
				Code: missingParameter,
			},
			{
				Rule: "the login banner's BannerText is up to 1400 characters",
				Doc:  page("ClientLoginBannerOptions"),
				Break: modifyEndpoint(func(in *awsec2.ModifyClientVpnEndpointInput) {
					in.ClientLoginBannerOptions = &types.ClientLoginBannerOptions{Enabled: aws.Bool(true), BannerText: aws.String(strings.Repeat("a", 1401))}
				}),
				Code: invalidValue,
			},
			{
				Rule: "DnsServers with Enabled false deletes the endpoint's DNS servers",
				Doc:  page("DnsServersOptionsModifyStructure"),
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := createEndpoint(ctx, f, func(in *awsec2.CreateClientVpnEndpointInput) { in.DnsServers = []string{"10.0.0.2"} })
					if err != nil {
						return err
					}
					in := &awsec2.ModifyClientVpnEndpointInput{ClientVpnEndpointId: aws.String(id), DnsServers: &types.DnsServersOptionsModifyStructure{Enabled: aws.Bool(false)}}
					if _, err := f.ModifyClientVpnEndpoint(ctx, in, inTestRegion); err != nil {
						return err
					}
					e, err := describeEndpoint(ctx, f, id)
					if err != nil {
						return err
					}
					if len(e.DnsServers) > 0 {
						return fmt.Errorf("the endpoint has the DNS servers %v", e.DnsServers)
					}
					return nil
				},
			},
		},
		Unclear: codes + " The page says TransitGatewayConfiguration is not supported, without an error; the handler " +
			"doesn't send it. It doesn't say that a Description of \"\" removes the description, which the handler " +
			"relies on and the fake does, or that the call keeps the values it doesn't have, which the fake does.",
	},
	"DeleteClientVpnEndpoint": {
		Doc: page("DeleteClientVpnEndpoint"),
		Rules: []nativetest.Rule{
			{
				Rule: "the endpoint exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteClientVpnEndpoint(ctx, &awsec2.DeleteClientVpnEndpointInput{ClientVpnEndpointId: aws.String(missingEndpoint)}, inTestRegion)
					return err
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
			{
				Rule: "a deleted endpoint is deleting, then deleted",
				Doc:  page("ClientVpnEndpointStatus"),
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := newEndpoint(ctx, f)
					if err != nil {
						return err
					}
					if _, err := f.DeleteClientVpnEndpoint(ctx, &awsec2.DeleteClientVpnEndpointInput{ClientVpnEndpointId: aws.String(id)}, inTestRegion); err != nil {
						return err
					}
					if err := endpointStatusIs(ctx, f, id, types.ClientVpnEndpointStatusCodeDeleting); err != nil {
						return err
					}
					return endpointStatusIs(ctx, f, id, types.ClientVpnEndpointStatusCodeDeleted)
				},
			},
		},
		Unclear: "The page says every target network must be disassociated before the delete, but names no error; " +
			"the fake doesn't enforce it. UCP deletes the associations, which depend on the endpoint, first.",
	},
	"GetClientVpnEndpointAuthorizationPolicy": {
		Doc: page("GetClientVpnEndpointAuthorizationPolicy"),
		Rules: []nativetest.Rule{
			{
				Rule: "the endpoint exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := getPolicy(ctx, f, missingEndpoint)
					return err
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
		},
		Unclear: "The page doesn't say what EC2 returns for an endpoint without a policy. The fake returns " +
			"InvalidClientVpnEndpointAuthorizationPolicy.NotFound, a code the error codes page doesn't list; the " +
			"handler reads any NotFound code, and a policy without a document, as no policy.",
	},
	"ModifyClientVpnEndpointAuthorizationPolicy": {
		Doc: page("ModifyClientVpnEndpointAuthorizationPolicy"),
		Rules: []nativetest.Rule{
			{
				Rule: "the endpoint exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					return setPolicy(ctx, f, missingEndpoint, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("permit(principal, action, resource);")})
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
			{
				Rule: "PolicyDocument is required for an endpoint that has no policy",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					return setPolicy(ctx, f, id, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{ShadowMode: types.ClientVpnAuthorizationPolicyShadowModeEnabled})
				}),
				Code: missingParameter,
			},
			{
				Rule: "ShadowMode is enabled or disabled",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					return setPolicy(ctx, f, id, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("permit(principal, action, resource);"), ShadowMode: "on"})
				}),
				Code: invalidValue,
			},
			{
				Rule: "ShadowMode defaults to disabled, and a call keeps the values of the policy it doesn't have",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					if err := setPolicy(ctx, f, id, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("permit(principal, action, resource);"), Description: aws.String("all")}); err != nil {
						return err
					}
					if err := setPolicy(ctx, f, id, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("forbid(principal, action, resource);")}); err != nil {
						return err
					}
					out, err := getPolicy(ctx, f, id)
					if err != nil {
						return err
					}
					if out.ShadowMode != types.ClientVpnAuthorizationPolicyShadowModeDisabled || aws.ToString(out.Description) != "all" {
						return fmt.Errorf("the policy has ShadowMode %q and Description %q", out.ShadowMode, aws.ToString(out.Description))
					}
					return nil
				},
			},
			{
				Rule: "a request sent again with the same client token and parameters changes nothing",
				Doc:  idempotency,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					first := func() *awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput {
						return &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("permit(principal, action, resource);"), ClientToken: aws.String("policy-token")}
					}
					if err := setPolicy(ctx, f, id, first()); err != nil {
						return err
					}
					if err := setPolicy(ctx, f, id, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("forbid(principal, action, resource);")}); err != nil {
						return err
					}
					if err := setPolicy(ctx, f, id, first()); err != nil {
						return err
					}
					out, err := getPolicy(ctx, f, id)
					if err != nil {
						return err
					}
					if !strings.HasPrefix(aws.ToString(out.PolicyDocument), "forbid") {
						return fmt.Errorf("the request sent again set the policy again")
					}
					return nil
				},
			},
			{
				Rule: "a client token that another call's request had fails, so a create's token can't set the endpoint's policy",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					id, err := createEndpoint(ctx, f, nil)
					if err != nil {
						return err
					}
					return setPolicy(ctx, f, id, &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{PolicyDocument: aws.String("permit(principal, action, resource);"), ClientToken: endpointInput(nil).ClientToken})
				}),
				Code: "IdempotentParameterMismatch",
			},
		},
		Unclear: codes + " The call isn't in the idempotency page's list, but its page links to it for ClientToken; the " +
			"fake applies the page's rules. The page doesn't say whether a new policy reports creating or updating; " +
			"the fake reports updating once. It doesn't say that a Description of \"\" removes the description, which " +
			"the handler relies on and the fake does.",
	},
	"DeleteClientVpnEndpointAuthorizationPolicy": {
		Doc: page("DeleteClientVpnEndpointAuthorizationPolicy"),
		Rules: []nativetest.Rule{
			{
				Rule: "the endpoint exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteClientVpnEndpointAuthorizationPolicy(ctx, &awsec2.DeleteClientVpnEndpointAuthorizationPolicyInput{ClientVpnEndpointId: aws.String(missingEndpoint)}, inTestRegion)
					return err
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
		},
		Unclear: "The page doesn't say what EC2 does for an endpoint without a policy; the fake succeeds.",
	},
}

// associationInput returns a request for an association EC2 takes, after change makes its changes, when it isn't
// nil.
func associationInput(change func(in *awsec2.AssociateClientVpnTargetNetworkInput)) *awsec2.AssociateClientVpnTargetNetworkInput {
	in := &awsec2.AssociateClientVpnTargetNetworkInput{
		ClientVpnEndpointId: aws.String("cvpn-endpoint-1"),
		SubnetId:            aws.String("subnet-1"),
		ClientToken:         aws.String("association-token"),
	}
	if change != nil {
		change(in)
	}
	return in
}

// associate associates a target network as associationInput says, and returns the association ID.
func associate(ctx context.Context, f *fakeAPI, change func(in *awsec2.AssociateClientVpnTargetNetworkInput)) (string, error) {
	out, err := f.AssociateClientVpnTargetNetwork(ctx, associationInput(change), inTestRegion)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.AssociationId), nil
}

// rejectAssociation returns a Break that associates a target network with the change.
func rejectAssociation(change func(in *awsec2.AssociateClientVpnTargetNetworkInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return breaks(func(ctx context.Context, f *fakeAPI) error {
		_, err := associate(ctx, f, change)
		return err
	})
}

// clientVpnTargetNetworkAssociationAPI records the EC2 calls ClientVpnTargetNetworkAssociation makes.
var clientVpnTargetNetworkAssociationAPI = map[string]nativetest.Call{
	"AssociateClientVpnTargetNetwork": {
		Doc: page("AssociateClientVpnTargetNetwork"),
		Rules: []nativetest.Rule{
			{
				Rule:  "SubnetId, for a VPC endpoint, or AvailabilityZone or AvailabilityZoneId, for a transit gateway endpoint, is required",
				Doc:   commonErrors,
				Break: rejectAssociation(func(in *awsec2.AssociateClientVpnTargetNetworkInput) { in.SubnetId = nil }),
				Code:  missingParameter,
			},
			{
				Rule:  "SubnetId and AvailabilityZone can't be combined",
				Doc:   commonErrors,
				Break: rejectAssociation(func(in *awsec2.AssociateClientVpnTargetNetworkInput) { in.AvailabilityZone = aws.String("us-west-2a") }),
				Code:  invalidCombination,
			},
			{
				Rule: "SubnetId can't be sent with AvailabilityZone, even an empty one",
				Doc:  commonErrors,
				Break: rejectAssociation(func(in *awsec2.AssociateClientVpnTargetNetworkInput) {
					in.SubnetId = aws.String("")
					in.AvailabilityZone = aws.String("us-west-2a")
				}),
				Code: invalidCombination,
			},
			{
				Rule: "AvailabilityZone and AvailabilityZoneId can't be combined",
				Doc:  commonErrors,
				Break: rejectAssociation(func(in *awsec2.AssociateClientVpnTargetNetworkInput) {
					in.SubnetId = nil
					in.AvailabilityZone = aws.String("us-west-2a")
					in.AvailabilityZoneId = aws.String("usw2-az1")
				}),
				Code: invalidCombination,
			},
			{
				Rule: "the endpoint exists",
				Doc:  commonErrors,
				Break: rejectAssociation(func(in *awsec2.AssociateClientVpnTargetNetworkInput) {
					in.ClientVpnEndpointId = aws.String(missingEndpoint)
				}),
				Code: "InvalidClientVpnEndpointId.NotFound",
			},
			{
				Rule: "a subnet already associated with the endpoint fails",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					if _, err := associate(ctx, f, nil); err != nil {
						return err
					}
					_, err := associate(ctx, f, func(in *awsec2.AssociateClientVpnTargetNetworkInput) { in.ClientToken = aws.String("other-token") })
					return err
				}),
				Code: "InvalidClientVpnDuplicateAssociationException",
			},
			{
				Rule: "a client token is up to 64 ASCII characters",
				Doc:  idempotency,
				Break: rejectAssociation(func(in *awsec2.AssociateClientVpnTargetNetworkInput) {
					in.ClientToken = aws.String(strings.Repeat("t", 65))
				}),
				Code: "InvalidClientToken",
			},
			{
				Rule: "a request sent again with the same client token and parameters returns the first association",
				Doc:  idempotency,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					first, err := associate(ctx, f, nil)
					if err != nil {
						return err
					}
					again, err := associate(ctx, f, nil)
					if err != nil {
						return err
					}
					if again != first {
						return fmt.Errorf("the request sent again made the association %s, not %s", again, first)
					}
					return nil
				},
			},
			{
				Rule: "a client token sent again with other parameters fails",
				Doc:  idempotency,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					if _, err := associate(ctx, f, nil); err != nil {
						return err
					}
					_, err := associate(ctx, f, func(in *awsec2.AssociateClientVpnTargetNetworkInput) { in.SubnetId = aws.String("subnet-2") })
					return err
				}),
				Code: "IdempotentParameterMismatch",
			},
		},
		Unclear: codes + " The fake has no subnets or Availability Zones, so it doesn't check that a subnet is in the " +
			"endpoint's VPC (InvalidClientVpnSubnetId.NotFound) or that each Availability Zone has one subnet " +
			"(InvalidClientVpnSubnetId.DuplicateAz). It doesn't keep endpoints for associations: only " +
			"cvpn-endpoint-missing doesn't exist.",
	},
	"DescribeClientVpnTargetNetworks": {
		Doc: page("DescribeClientVpnTargetNetworks"),
		Rules: []nativetest.Rule{
			{
				Rule: "ClientVpnEndpointId is required",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeClientVpnTargetNetworks(ctx, &awsec2.DescribeClientVpnTargetNetworksInput{}, inTestRegion)
					return err
				}),
				Code: missingParameter,
			},
			{
				Rule: "MaxResults is 5 to 1000",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeClientVpnTargetNetworks(ctx, &awsec2.DescribeClientVpnTargetNetworksInput{ClientVpnEndpointId: aws.String("cvpn-endpoint-1"), MaxResults: aws.Int32(1001)}, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "MaxResults can't be sent with association IDs",
				Doc:  pagination,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					association, err := associate(ctx, f, nil)
					if err != nil {
						return err
					}
					in := &awsec2.DescribeClientVpnTargetNetworksInput{ClientVpnEndpointId: aws.String("cvpn-endpoint-1"), AssociationIds: []string{association}, MaxResults: aws.Int32(5)}
					_, err = f.DescribeClientVpnTargetNetworks(ctx, in, inTestRegion)
					return err
				}),
				Code: invalidCombination,
			},
			{
				Rule: "an association that doesn't exist fails",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := &awsec2.DescribeClientVpnTargetNetworksInput{ClientVpnEndpointId: aws.String("cvpn-endpoint-1"), AssociationIds: []string{"cvpn-assoc-404"}}
					_, err := f.DescribeClientVpnTargetNetworks(ctx, in, inTestRegion)
					return err
				}),
				Code: "InvalidClientVpnAssociationIdNotFound",
			},
		},
		Unclear: "The page doesn't say how long EC2 lists a disassociated association; the fake lists it once.",
	},
	"DisassociateClientVpnTargetNetwork": {
		Doc: page("DisassociateClientVpnTargetNetwork"),
		Rules: []nativetest.Rule{
			{
				Rule: "the association exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := &awsec2.DisassociateClientVpnTargetNetworkInput{ClientVpnEndpointId: aws.String("cvpn-endpoint-1"), AssociationId: aws.String("cvpn-assoc-404")}
					_, err := f.DisassociateClientVpnTargetNetwork(ctx, in, inTestRegion)
					return err
				}),
				Code: "InvalidClientVpnAssociationIdNotFound",
			},
			{
				Rule: "once the last target network is disassociated, the endpoint is pending-associate",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := newEndpoint(ctx, f)
					if err != nil {
						return err
					}
					association, err := associate(ctx, f, func(in *awsec2.AssociateClientVpnTargetNetworkInput) { in.ClientVpnEndpointId = aws.String(id) })
					if err != nil {
						return err
					}
					if err := endpointStatusIs(ctx, f, id, types.ClientVpnEndpointStatusCodeAvailable); err != nil {
						return err
					}
					in := &awsec2.DisassociateClientVpnTargetNetworkInput{ClientVpnEndpointId: aws.String(id), AssociationId: aws.String(association)}
					if _, err := f.DisassociateClientVpnTargetNetwork(ctx, in, inTestRegion); err != nil {
						return err
					}
					return endpointStatusIs(ctx, f, id, types.ClientVpnEndpointStatusCodePendingAssociate)
				},
			},
		},
	},
}

// validEntryInput returns an entry EC2 takes, with rule number 100 of the ingress rules of acl-1.
func validEntryInput() *awsec2.CreateNetworkAclEntryInput {
	return &awsec2.CreateNetworkAclEntryInput{
		NetworkAclId: aws.String("acl-1"),
		RuleNumber:   aws.Int32(100),
		Egress:       aws.Bool(false),
		RuleAction:   types.RuleActionAllow,
		Protocol:     aws.String("6"),
		CidrBlock:    aws.String("10.0.0.0/16"),
		PortRange:    &types.PortRange{From: aws.Int32(443), To: aws.Int32(443)},
	}
}

// putEntry creates or replaces an entry with the values of e, other than its rule number and direction.
type putEntry func(ctx context.Context, f *fakeAPI, e types.NetworkAclEntry) error

// createEntry creates rule 100 of acl-1 with the values of e.
func createEntry(ctx context.Context, f *fakeAPI, e types.NetworkAclEntry) error {
	in := validEntryInput()
	in.RuleAction, in.Protocol, in.CidrBlock, in.Ipv6CidrBlock, in.PortRange, in.IcmpTypeCode = e.RuleAction, e.Protocol, e.CidrBlock, e.Ipv6CidrBlock, e.PortRange, e.IcmpTypeCode
	_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
	return err
}

// replaceEntry creates rule 100 of acl-1, then replaces it with the values of e.
func replaceEntry(ctx context.Context, f *fakeAPI, e types.NetworkAclEntry) error {
	if _, err := f.CreateNetworkAclEntry(ctx, validEntryInput(), inTestRegion); err != nil {
		return err
	}
	_, err := f.ReplaceNetworkAclEntry(ctx, &awsec2.ReplaceNetworkAclEntryInput{
		NetworkAclId: aws.String("acl-1"), RuleNumber: aws.Int32(100), Egress: aws.Bool(false),
		RuleAction: e.RuleAction, Protocol: e.Protocol, CidrBlock: e.CidrBlock, Ipv6CidrBlock: e.Ipv6CidrBlock, PortRange: e.PortRange, IcmpTypeCode: e.IcmpTypeCode,
	}, inTestRegion)
	return err
}

// rejectEntry returns a Break that puts an entry with the values of validEntryInput, changed as change says.
func rejectEntry(put putEntry, change func(e *types.NetworkAclEntry)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return breaks(func(ctx context.Context, f *fakeAPI) error {
		in := validEntryInput()
		e := types.NetworkAclEntry{RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, PortRange: in.PortRange}
		change(&e)
		return put(ctx, f, e)
	})
}

// entryRulesOf returns the rules EC2's API reference gives for the entry of a call that creates or replaces one.
func entryRulesOf(put putEntry) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "RuleAction is allow or deny",
			Doc:   commonErrors,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) { e.RuleAction = "permit" }),
			Code:  invalidValue,
		},
		{
			Rule:  "Protocol is -1, for all protocols, or a protocol number",
			Doc:   naclRules,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) { e.Protocol = aws.String("256") }),
			Code:  invalidValue,
		},
		{
			Rule:  "TCP (6) and UDP (17) need PortRange",
			Doc:   commonErrors,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) { e.Protocol, e.PortRange = aws.String("17"), nil }),
			Code:  missingParameter,
		},
		{
			Rule:  "ICMP (1) needs Icmp",
			Doc:   commonErrors,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) { e.Protocol, e.PortRange = aws.String("1"), nil }),
			Code:  missingParameter,
		},
		{
			Rule: "ICMPv6 (58) with an IPv6 block needs Icmp",
			Doc:  commonErrors,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) {
				e.Protocol, e.PortRange, e.CidrBlock, e.Ipv6CidrBlock = aws.String("58"), nil, nil, aws.String("::/0")
			}),
			Code: missingParameter,
		},
		{
			Rule:  "CidrBlock or Ipv6CidrBlock is required",
			Doc:   naclEntryTemplate,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) { e.CidrBlock = nil }),
			Code:  missingParameter,
		},
		{
			Rule:  "CidrBlock is an IPv4 block and Ipv6CidrBlock an IPv6 block, in CIDR notation",
			Doc:   commonErrors,
			Break: rejectEntry(put, func(e *types.NetworkAclEntry) { e.CidrBlock = aws.String("2001:db8::/32") }),
			Code:  invalidValue,
		},
	}
}

// describeAcl returns the network ACL with the ID.
func describeAcl(ctx context.Context, f *fakeAPI, acl string) (types.NetworkAcl, error) {
	out, err := f.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{NetworkAclIds: []string{acl}}, inTestRegion)
	if err != nil {
		return types.NetworkAcl{}, err
	}
	return out.NetworkAcls[0], nil
}

// entryOfAcl returns the entry of the network ACL with the rule number and direction, or nil.
func entryOfAcl(acl types.NetworkAcl, rule int32, egress bool) *types.NetworkAclEntry {
	for _, e := range acl.Entries {
		if aws.ToInt32(e.RuleNumber) == rule && aws.ToBool(e.Egress) == egress {
			return &e
		}
	}
	return nil
}

// networkAclEntryAPI records the EC2 calls NetworkAclEntry makes.
var networkAclEntryAPI = map[string]nativetest.Call{
	"CreateNetworkAclEntry": {
		Doc: page("CreateNetworkAclEntry"),
		Rules: append(entryRulesOf(createEntry),
			nativetest.Rule{
				Rule: "RuleNumber is 1 to 32766; EC2 keeps 32767 to 65535 for its own rules",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := validEntryInput()
					in.RuleNumber = aws.Int32(32767)
					_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			nativetest.Rule{
				Rule: "a RuleNumber of 0 isn't 1 to 32766",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := validEntryInput()
					in.RuleNumber = aws.Int32(0)
					_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			nativetest.Rule{
				Rule: "NetworkAclId, RuleNumber, and Egress are required",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := validEntryInput()
					in.Egress = nil
					_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
					return err
				}),
				Code: missingParameter,
			},
			nativetest.Rule{
				Rule: "EC2 keeps CidrBlock in its canonical form, so 100.68.0.18/18 is 100.68.0.0/18",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					in := validEntryInput()
					in.CidrBlock = aws.String("100.68.0.18/18")
					if _, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion); err != nil {
						return err
					}
					acl, err := describeAcl(ctx, f, "acl-1")
					if err != nil {
						return err
					}
					if e := entryOfAcl(acl, 100, false); e == nil || aws.ToString(e.CidrBlock) != "100.68.0.0/18" {
						return fmt.Errorf("the entry is %+v", e)
					}
					return nil
				},
			},
			nativetest.Rule{
				Rule: "a rule number is used once in a direction of a network ACL",
				Doc:  naclEntryTemplate,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					if _, err := f.CreateNetworkAclEntry(ctx, validEntryInput(), inTestRegion); err != nil {
						return err
					}
					in := validEntryInput()
					in.RuleAction = types.RuleActionDeny
					_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
					return err
				}),
				Code: "NetworkAclEntryAlreadyExists",
			},
			nativetest.Rule{
				Rule: "an ingress rule and an egress rule can have the same number",
				Doc:  naclEntryTemplate,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					if _, err := f.CreateNetworkAclEntry(ctx, validEntryInput(), inTestRegion); err != nil {
						return err
					}
					in := validEntryInput()
					in.Egress = aws.Bool(true)
					_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
					return err
				},
			},
			nativetest.Rule{
				Rule: "the network ACL exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := validEntryInput()
					in.NetworkAclId = aws.String("acl-missing")
					_, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion)
					return err
				}),
				Code: "InvalidNetworkAclID.NotFound",
			},
		),
		Unclear: codes + " No page says whether CidrBlock and Ipv6CidrBlock can be sent together; the fake takes both. " +
			"The fake takes a protocol that isn't a number, such as tcp, which the page doesn't say EC2 takes, and keeps " +
			"the ports and ICMP values of a protocol that ignores them. It doesn't enforce the quota of rules per network " +
			"ACL (NetworkAclEntryLimitExceeded), 20 by default, because an account can raise it.",
	},
	"ReplaceNetworkAclEntry": {
		Doc: page("ReplaceNetworkAclEntry"),
		Rules: append(entryRulesOf(replaceEntry),
			nativetest.Rule{
				Rule: "the entry exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := validEntryInput()
					_, err := f.ReplaceNetworkAclEntry(ctx, &awsec2.ReplaceNetworkAclEntryInput{
						NetworkAclId: in.NetworkAclId, RuleNumber: in.RuleNumber, Egress: in.Egress, RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, PortRange: in.PortRange,
					}, inTestRegion)
					return err
				}),
				Code: "InvalidNetworkAclEntry.NotFound",
			},
			nativetest.Rule{
				Rule: "NetworkAclId and RuleNumber are required",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					in := validEntryInput()
					if _, err := f.CreateNetworkAclEntry(ctx, in, inTestRegion); err != nil {
						return err
					}
					_, err := f.ReplaceNetworkAclEntry(ctx, &awsec2.ReplaceNetworkAclEntryInput{
						NetworkAclId: in.NetworkAclId, Egress: in.Egress, RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, PortRange: in.PortRange,
					}, inTestRegion)
					return err
				}),
				Code: missingParameter,
			},
			nativetest.Rule{
				Rule: "without Egress, the call replaces the ingress rule",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					if _, err := f.CreateNetworkAclEntry(ctx, validEntryInput(), inTestRegion); err != nil {
						return err
					}
					in := validEntryInput()
					_, err := f.ReplaceNetworkAclEntry(ctx, &awsec2.ReplaceNetworkAclEntryInput{
						NetworkAclId: in.NetworkAclId, RuleNumber: in.RuleNumber, RuleAction: types.RuleActionDeny, Protocol: in.Protocol, CidrBlock: in.CidrBlock, PortRange: in.PortRange,
					}, inTestRegion)
					if err != nil {
						return err
					}
					acl, err := describeAcl(ctx, f, "acl-1")
					if err != nil {
						return err
					}
					if e := entryOfAcl(acl, 100, false); e == nil || e.RuleAction != types.RuleActionDeny {
						return fmt.Errorf("the ingress rule is %+v", e)
					}
					return nil
				},
			},
		),
		Unclear: codes + " Unlike CreateNetworkAclEntry's page, the page doesn't say EC2 keeps CidrBlock in its canonical " +
			"form; the fake keeps it as sent. The VPC guide says the rules numbered * (32767) can't be changed, without " +
			"an error; the fake doesn't keep them, so it returns InvalidNetworkAclEntry.NotFound.",
	},
	"DeleteNetworkAclEntry": {
		Doc: page("DeleteNetworkAclEntry"),
		Rules: []nativetest.Rule{
			{
				Rule: "the entry exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DeleteNetworkAclEntry(ctx, &awsec2.DeleteNetworkAclEntryInput{NetworkAclId: aws.String("acl-1"), RuleNumber: aws.Int32(100), Egress: aws.Bool(false)}, inTestRegion)
					return err
				}),
				Code: "InvalidNetworkAclEntry.NotFound",
			},
			{
				Rule: "NetworkAclId, RuleNumber, and Egress are required",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					if _, err := f.CreateNetworkAclEntry(ctx, validEntryInput(), inTestRegion); err != nil {
						return err
					}
					_, err := f.DeleteNetworkAclEntry(ctx, &awsec2.DeleteNetworkAclEntryInput{NetworkAclId: aws.String("acl-1"), RuleNumber: aws.Int32(100)}, inTestRegion)
					return err
				}),
				Code: missingParameter,
			},
		},
		Unclear: "The VPC guide says a rule numbered * (32767) can't be deleted, without an error; the fake doesn't keep " +
			"those rules, so it returns InvalidNetworkAclEntry.NotFound.",
	},
	"DescribeNetworkAcls": {
		Doc: page("DescribeNetworkAcls"),
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 5 to 1000",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{MaxResults: aws.Int32(4)}, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "MaxResults can't be sent with network ACL IDs",
				Doc:  pagination,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeNetworkAcls(ctx, &awsec2.DescribeNetworkAclsInput{NetworkAclIds: []string{"acl-1"}, MaxResults: aws.Int32(5)}, inTestRegion)
					return err
				}),
				Code: invalidCombination,
			},
			{
				Rule: "a network ACL that doesn't exist fails",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := describeAcl(ctx, f, "acl-missing")
					return err
				}),
				Code: "InvalidNetworkAclID.NotFound",
			},
			{
				Rule: "each network ACL has a rule numbered * (32767) in each direction that denies all traffic",
				Doc:  defaultNacl,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					acl, err := describeAcl(ctx, newFakeAPI(store), "acl-1")
					if err != nil {
						return err
					}
					for _, egress := range []bool{false, true} {
						if e := entryOfAcl(acl, 32767, egress); e == nil || e.RuleAction != types.RuleActionDeny {
							return fmt.Errorf("the rule 32767 with egress %t is %+v", egress, e)
						}
					}
					return nil
				},
			},
		},
	},
}

// grant creates a permission for the network interface, and returns its ID.
func grant(ctx context.Context, f *fakeAPI, networkInterface string, permission types.InterfacePermissionType) (string, error) {
	out, err := f.CreateNetworkInterfacePermission(ctx, &awsec2.CreateNetworkInterfacePermissionInput{
		NetworkInterfaceId: aws.String(networkInterface), AwsAccountId: aws.String("111122223333"), Permission: permission,
	}, inTestRegion)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.InterfacePermission.NetworkInterfacePermissionId), nil
}

// networkInterfacePermissionAPI records the EC2 calls NetworkInterfacePermission makes.
var networkInterfacePermissionAPI = map[string]nativetest.Call{
	"CreateNetworkInterfacePermission": {
		Doc: page("CreateNetworkInterfacePermission"),
		Rules: []nativetest.Rule{
			{
				Rule: "Permission is INSTANCE-ATTACH or EIP-ASSOCIATE",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := grant(ctx, f, "eni-1", "INSTANCE-DETACH")
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "the network interface exists",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := grant(ctx, f, missingInterface, types.InterfacePermissionTypeInstanceAttach)
					return err
				}),
				Code: "InvalidNetworkInterfaceID.NotFound",
			},
		},
		Unclear: codes + " The call takes no client token, and the page doesn't say what EC2 does with a second grant " +
			"of the same permission; the fake grants it again. AwsService is not supported, and the handler doesn't send it.",
	},
	"DescribeNetworkInterfacePermissions": {
		Doc: page("DescribeNetworkInterfacePermissions"),
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 5 to 255",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeNetworkInterfacePermissions(ctx, &awsec2.DescribeNetworkInterfacePermissionsInput{MaxResults: aws.Int32(256)}, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "MaxResults can't be sent with permission IDs",
				Doc:  pagination,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					id, err := grant(ctx, f, "eni-1", types.InterfacePermissionTypeInstanceAttach)
					if err != nil {
						return err
					}
					_, err = f.DescribeNetworkInterfacePermissions(ctx, &awsec2.DescribeNetworkInterfacePermissionsInput{NetworkInterfacePermissionIds: []string{id}, MaxResults: aws.Int32(5)}, inTestRegion)
					return err
				}),
				Code: invalidCombination,
			},
			{
				Rule: "without MaxResults, a call returns up to 50 permissions, and a NextToken for the rest",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range 51 {
						if _, err := grant(ctx, f, fmt.Sprintf("eni-%d", i), types.InterfacePermissionTypeInstanceAttach); err != nil {
							return err
						}
					}
					first, err := f.DescribeNetworkInterfacePermissions(ctx, &awsec2.DescribeNetworkInterfacePermissionsInput{}, inTestRegion)
					if err != nil {
						return err
					}
					if len(first.NetworkInterfacePermissions) != 50 || first.NextToken == nil {
						return fmt.Errorf("the first page has %d permissions and NextToken %v", len(first.NetworkInterfacePermissions), first.NextToken)
					}
					rest, err := f.DescribeNetworkInterfacePermissions(ctx, &awsec2.DescribeNetworkInterfacePermissionsInput{NextToken: first.NextToken}, inTestRegion)
					if err != nil {
						return err
					}
					if len(rest.NetworkInterfacePermissions) != 1 || rest.NextToken != nil {
						return fmt.Errorf("the second page has %d permissions and NextToken %v", len(rest.NetworkInterfacePermissions), rest.NextToken)
					}
					return nil
				},
			},
		},
		Unclear: "Neither the page nor the error codes page names the error for a permission ID that doesn't exist. " +
			"The fake returns InvalidPermissionId.NotFound, one of the codes the handler reads as not found. The page " +
			"doesn't say how long EC2 lists a revoked permission; the fake keeps listing it.",
	},
	"DeleteNetworkInterfacePermission": {
		Doc: page("DeleteNetworkInterfacePermission"),
		Unclear: "Neither the page nor the error codes page names the error for a permission ID that doesn't exist; the " +
			"fake returns InvalidPermissionId.NotFound. The page says EC2 doesn't delete a permission whose network " +
			"interface the other account attached unless Force is true, without an error; the fake has no attachments, " +
			"and the handler doesn't send Force.",
	},
}

// propagate turns on or off propagation from the gateway to the route table.
func propagate(on bool) func(ctx context.Context, f *fakeAPI, gateway string, table string) error {
	return func(ctx context.Context, f *fakeAPI, gateway string, table string) error {
		if on {
			_, err := f.EnableVgwRoutePropagation(ctx, &awsec2.EnableVgwRoutePropagationInput{GatewayId: aws.String(gateway), RouteTableId: aws.String(table)}, inTestRegion)
			return err
		}
		_, err := f.DisableVgwRoutePropagation(ctx, &awsec2.DisableVgwRoutePropagationInput{GatewayId: aws.String(gateway), RouteTableId: aws.String(table)}, inTestRegion)
		return err
	}
}

// propagationRulesOf returns the rules EC2's error codes page gives for the gateway and route table of a call.
func propagationRulesOf(call func(ctx context.Context, f *fakeAPI, gateway string, table string) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "the gateway exists",
			Doc:  commonErrors,
			Break: breaks(func(ctx context.Context, f *fakeAPI) error {
				return call(ctx, f, "gw-missing", "rtb-1")
			}),
			Code: "InvalidVpnGatewayID.NotFound",
		},
		{
			Rule: "the route table exists",
			Doc:  commonErrors,
			Break: breaks(func(ctx context.Context, f *fakeAPI) error {
				return call(ctx, f, "vgw-1", "table-missing")
			}),
			Code: "InvalidRouteTableID.NotFound",
		},
	}
}

// vpnGatewayRoutePropagationAPI records the EC2 calls VPNGatewayRoutePropagation makes.
var vpnGatewayRoutePropagationAPI = map[string]nativetest.Call{
	"EnableVgwRoutePropagation": {
		Doc: page("EnableVgwRoutePropagation"),
		Rules: append(propagationRulesOf(propagate(true)), nativetest.Rule{
			Rule: "the gateway is attached to the VPC of the route table",
			Doc:  commonErrors,
			Break: breaks(func(ctx context.Context, f *fakeAPI) error {
				return propagate(true)(ctx, f, detachedGateway+"-1", "rtb-1")
			}),
			Code: "InvalidVpnGatewayAttachment.NotFound",
		}),
		Unclear: "The fake has no gateways or route tables: the IDs with the prefixes vgw- and rtb- exist, and every " +
			"route table is in vpc-1, which every gateway without the prefix vgw-detached is attached to. The page " +
			"doesn't say what EC2 does for a route table that already has propagation from the gateway; the fake " +
			"changes nothing, as the handler's recovery by resend expects.",
	},
	"DisableVgwRoutePropagation": {
		Doc:   page("DisableVgwRoutePropagation"),
		Rules: propagationRulesOf(propagate(false)),
		Unclear: "The page doesn't say what EC2 does for a route table that has no propagation from the gateway; the " +
			"fake changes nothing.",
	},
	"DescribeRouteTables": {
		Doc: page("DescribeRouteTables"),
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is 5 to 100",
				Doc:  commonErrors,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{MaxResults: aws.Int32(101)}, inTestRegion)
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "MaxResults can't be sent with route table IDs",
				Doc:  pagination,
				Break: breaks(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{RouteTableIds: []string{"rtb-1"}, MaxResults: aws.Int32(5)}, inTestRegion)
					return err
				}),
				Code: invalidCombination,
			},
		},
		Unclear: "The page doesn't say that the route-table-id filter, unlike RouteTableIds, leaves out a route table " +
			"that doesn't exist, which the handler relies on and the fake does. The fake takes only that filter, and " +
			"returns InvalidParameterValue for any other, which the page doesn't say.",
	},
}
