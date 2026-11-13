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
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// outsidePropagationType is the pseudo type under which the fake keeps, per gateway, the route tables a gateway
// propagates to that were turned on outside Radius.
const outsidePropagationType = "outside::" + vpnGatewayRoutePropagationType

// fakeAPI is an in-memory EC2 API. It keeps each type's resources in an awsfake store, in the type's property format,
// so the suite's Cloud Control sees the same resources. A Client VPN endpoint, a target network association, and a
// network interface permission report a transitional status for one read after a create or delete.
//
// For route propagation, the store has one entry per identifier with the route tables that alias turned on, and one
// entry per gateway under outsidePropagationType with the ones turned on outside Radius. A gateway propagates to the
// union. Turning propagation on adds the route table to the current entry, which the test sets, and turning it off
// takes the route table out of every entry of the gateway, as AWS does.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu      sync.Mutex
	next    int
	script  map[string][]string
	revoked []types.NetworkInterfacePermission
	calls   []string

	// tokens are the requests made with a client token, by region and token.
	tokens map[string]tokenUse

	// acls are the network ACLs that exist, besides the ones that have entries in the store.
	acls []string

	// current is the identifier of the route propagation entry a call that turns propagation on adds to.
	current string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, script: map[string][]string{}, tokens: map[string]tokenUse{}, errs: map[string]error{}, acls: []string{"acl-1", "acl-2"}}
}

// The error codes EC2's error codes page gives for a request with a value that isn't valid, a missing parameter, and
// parameters that can't be combined.
const (
	invalidValue       = "InvalidParameterValue"
	missingParameter   = "MissingParameter"
	invalidCombination = "InvalidParameterCombination"
)

// The IDs that don't exist in the fake: an endpoint and a network interface. A gateway with the detached prefix is
// attached to no VPC.
const (
	missingEndpoint  = "cvpn-endpoint-missing"
	missingInterface = "eni-missing"
	detachedGateway  = "vgw-detached"
)

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func regionOf(opts []func(*awsec2.Options)) string {
	options := awsec2.Options{}
	for _, fn := range opts {
		fn(&options)
	}
	return options.Region
}

// call records a call, and returns the error errs sets for it. A call without a region fails, as with AWS.
func (f *fakeAPI) call(name string, opts []func(*awsec2.Options)) error {
	f.calls = append(f.calls, name)
	if regionOf(opts) == "" {
		return fmt.Errorf("the call %s has no region", name)
	}
	return f.errs[name]
}

// oneOf returns InvalidParameterValue, as EC2 does, when value is set and isn't one of valid. It is for enum values,
// which the SDK leaves "" when unset.
func oneOf[T ~string](value T, valid ...T) error {
	if value != "" && !slices.Contains(valid, value) {
		return fault(invalidValue)
	}
	return nil
}

// oneOfNumber returns InvalidParameterValue, as EC2 does, when value is set, even to 0, and isn't one of valid.
func oneOfNumber(value *int32, valid ...int32) error {
	if value != nil && !slices.Contains(valid, *value) {
		return fault(invalidValue)
	}
	return nil
}

// validPage returns the error EC2 returns for the MaxResults of a describe call that takes from low to high, sent
// with ids IDs: EC2 rejects MaxResults with a list of IDs.
func validPage(maxResults *int32, ids int, low int32, high int32) error {
	if maxResults == nil {
		return nil
	}
	if ids > 0 {
		return fault(invalidCombination)
	}
	if *maxResults < low || *maxResults > high {
		return fault(invalidValue)
	}
	return nil
}

// tokenUse is a request EC2 got with a client token: the call, the request's other parameters, and the ID of what the
// request made.
type tokenUse struct {
	key     string
	call    string
	request map[string]any
	made    string
}

// maxTokenLength is the most ASCII characters EC2 takes in a client token.
const maxTokenLength = 64

// useToken applies EC2's client token rules to a request with token in region. It returns the request's use of the
// token, or nil when the request has no token, and the earlier use when an identical request had the token. A token
// that isn't up to 64 ASCII characters fails with InvalidClientToken, and one an earlier request of another call or
// with other parameters had fails with IdempotentParameterMismatch.
func (f *fakeAPI) useToken(call string, region string, token *string, in any) (*tokenUse, *tokenUse, error) {
	t := aws.ToString(token)
	if t == "" {
		return nil, nil, nil
	}
	if len(t) > maxTokenLength || strings.ContainsFunc(t, func(r rune) bool { return r > unicode.MaxASCII }) {
		return nil, nil, fault("InvalidClientToken")
	}
	request, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, nil, err
	}
	delete(request, "ClientToken")
	use := &tokenUse{key: region + "/" + t, call: call, request: request}
	earlier, ok := f.tokens[use.key]
	if !ok {
		return use, nil, nil
	}
	if earlier.call != call || !reflect.DeepEqual(earlier.request, request) {
		return nil, nil, fault("IdempotentParameterMismatch")
	}
	return use, &earlier, nil
}

// remember keeps the use of a client token, with the ID of what the request made.
func (f *fakeAPI) remember(use *tokenUse, made string) {
	if use != nil {
		use.made = made
		f.tokens[use.key] = *use
	}
}

func (f *fakeAPI) setCurrent(identifier string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = identifier
}

// newID returns a new ID with the prefix, such as cvpn-endpoint-1.
func (f *fakeAPI) newID(prefix string) string {
	f.next++
	return prefix + strconv.Itoa(f.next)
}

// status returns the status of the resource for a read, or final when no transitional status is left. A scripted
// status that ends with "!" is the last: the resource leaves the store once it is read.
func (f *fakeAPI) status(typeName string, id string, final string) string {
	key := typeName + "/" + id
	script := f.script[key]
	if len(script) == 0 {
		return final
	}
	f.script[key] = script[1:]
	if last, ok := strings.CutSuffix(script[0], "!"); ok {
		delete(f.script, key)
		f.store.Remove(typeName, id)
		return last
	}
	return script[0]
}

// change runs a call that changes AWS, with the failer.
func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

// The values EC2 takes for a Client VPN endpoint's session timeout and port, and the most DNS servers and banner
// characters it takes.
var (
	sessionTimeouts = []int32{8, 10, 12, 24}
	vpnPorts        = []int32{443, 1194}
)

const (
	maxDNSServers = 2
	maxBannerText = 1400
)

// validEndpointOptions returns the error EC2 returns for options that CreateClientVpnEndpoint and
// ModifyClientVpnEndpoint both take.
func validEndpointOptions(logs *types.ConnectionLogOptions, banner *types.ClientLoginBannerOptions, dnsServers []string, portal types.SelfServicePortal, timeout *int32, port *int32) error {
	if logs != nil && aws.ToBool(logs.Enabled) && aws.ToString(logs.CloudwatchLogGroup) == "" {
		return fault(missingParameter)
	}
	if banner != nil && utf8.RuneCountInString(aws.ToString(banner.BannerText)) > maxBannerText {
		return fault(invalidValue)
	}
	if len(dnsServers) > maxDNSServers {
		return fault(invalidValue)
	}
	return cmp.Or(
		oneOf(portal, types.SelfServicePortalEnabled, types.SelfServicePortalDisabled),
		oneOfNumber(timeout, sessionTimeouts...),
		oneOfNumber(port, vpnPorts...),
	)
}

// validEndpoint returns the error EC2 returns for an endpoint CreateClientVpnEndpoint rejects.
func validEndpoint(in *awsec2.CreateClientVpnEndpointInput) error {
	if len(in.AuthenticationOptions) == 0 || in.ConnectionLogOptions == nil || aws.ToString(in.ServerCertificateArn) == "" {
		return fault(missingParameter)
	}
	for _, a := range in.AuthenticationOptions {
		missing := false
		switch a.Type {
		case types.ClientVpnAuthenticationTypeCertificateAuthentication:
			missing = a.MutualAuthentication == nil
		case types.ClientVpnAuthenticationTypeDirectoryServiceAuthentication:
			missing = a.ActiveDirectory == nil
		case types.ClientVpnAuthenticationTypeFederatedAuthentication:
			missing = a.FederatedAuthentication == nil
		default:
			return fault(invalidValue)
		}
		if missing {
			return fault(missingParameter)
		}
	}
	if in.ClientCidrBlock != nil {
		prefix, err := netip.ParsePrefix(*in.ClientCidrBlock)
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 12 || prefix.Bits() > 22 {
			return fault(invalidValue)
		}
	}
	if len(in.SecurityGroupIds) > 0 && aws.ToString(in.VpcId) == "" {
		return fault(missingParameter)
	}
	if tgw := in.TransitGatewayConfiguration; tgw != nil {
		if in.VpcId != nil || len(in.SecurityGroupIds) > 0 || (len(tgw.AvailabilityZones) > 0 && len(tgw.AvailabilityZoneIds) > 0) {
			return fault(invalidCombination)
		}
	}
	for _, spec := range in.TagSpecifications {
		if spec.ResourceType != types.ResourceTypeClientVpnEndpoint {
			return fault(invalidValue)
		}
	}
	ipTypes := []string{"ipv4", "ipv6", "dual-stack"}
	return cmp.Or(
		validEndpointOptions(in.ConnectionLogOptions, in.ClientLoginBannerOptions, in.DnsServers, in.SelfServicePortal, in.SessionTimeoutHours, in.VpnPort),
		oneOf(string(in.EndpointIpAddressType), ipTypes...),
		oneOf(string(in.TrafficIpAddressType), ipTypes...),
		oneOf(in.TransportProtocol, types.TransportProtocolTcp, types.TransportProtocolUdp),
	)
}

// CreateClientVpnEndpoint creates an endpoint. When the VPC is set without security groups, the endpoint gets the
// VPC's default security group, as with AWS.
func (f *fakeAPI) CreateClientVpnEndpoint(ctx context.Context, in *awsec2.CreateClientVpnEndpointInput, opts ...func(*awsec2.Options)) (*awsec2.CreateClientVpnEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateClientVpnEndpoint", opts); err != nil {
		return nil, err
	}
	if err := validEndpoint(in); err != nil {
		return nil, err
	}
	use, earlier, err := f.useToken("CreateClientVpnEndpoint", regionOf(opts), in.ClientToken, in)
	if err != nil {
		return nil, err
	}
	if earlier != nil && f.store.Resource(clientVpnEndpointType, earlier.made) != nil {
		return &awsec2.CreateClientVpnEndpointOutput{ClientVpnEndpointId: aws.String(earlier.made)}, nil
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	delete(props, "ClientToken")
	if vpc := aws.ToString(in.VpcId); vpc != "" && len(in.SecurityGroupIds) == 0 {
		props["SecurityGroupIds"] = []any{defaultSecurityGroup(vpc)}
	}
	id := f.newID("cvpn-endpoint-")
	props["Id"] = id
	f.store.Put(clientVpnEndpointType, id, props)
	f.script[clientVpnEndpointType+"/"+id] = []string{"pending"}
	f.remember(use, id)
	return &awsec2.CreateClientVpnEndpointOutput{ClientVpnEndpointId: aws.String(id)}, nil
}

// defaultSecurityGroup returns the ID of the VPC's default security group in the fake.
func defaultSecurityGroup(vpc string) string {
	return "sg-default-" + vpc
}

// endpointStatus returns the final status of the endpoint: available when a target network is associated with it,
// and pending-associate otherwise, as with AWS.
func (f *fakeAPI) endpointStatus(id string) string {
	for _, key := range f.store.List(clientVpnTargetNetworkAssociationType) {
		leaving := slices.ContainsFunc(f.script[clientVpnTargetNetworkAssociationType+"/"+key], func(s string) bool { return strings.HasSuffix(s, "!") })
		if !leaving && awsapi.String(f.store.Resource(clientVpnTargetNetworkAssociationType, key), "ClientVpnEndpointId") == id {
			return string(types.ClientVpnEndpointStatusCodeAvailable)
		}
	}
	return string(types.ClientVpnEndpointStatusCodePendingAssociate)
}

// endpoint returns the endpoint the store has under id, with the defaults AWS sets, in the API's format.
func (f *fakeAPI) endpoint(id string, props map[string]any) (types.ClientVpnEndpoint, error) {
	e := types.ClientVpnEndpoint{
		VpnPort:                    aws.Int32(443),
		SessionTimeoutHours:        aws.Int32(24),
		SplitTunnel:                aws.Bool(false),
		TransportProtocol:          types.TransportProtocolUdp,
		DisconnectOnSessionTimeout: aws.Bool(true),
		EndpointIpAddressType:      types.EndpointIpAddressTypeIpv4,
		TrafficIpAddressType:       types.TrafficIpAddressTypeIpv4,
	}
	if err := awsapi.Decode(awsapi.Without(props, "AuthenticationOptions", "TagSpecifications"), &e); err != nil {
		return e, err
	}
	e.ClientVpnEndpointId = aws.String(id)
	e.Status = &types.ClientVpnEndpointStatus{Code: types.ClientVpnEndpointStatusCode(f.status(clientVpnEndpointType, id, f.endpointStatus(id)))}
	if awsapi.String(props, "SelfServicePortal") == string(types.SelfServicePortalEnabled) {
		e.SelfServicePortalUrl = aws.String("https://self-service.clientvpn.amazonaws.com/endpoints/" + id)
	}
	list, _ := props["AuthenticationOptions"].([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		a := types.ClientVpnAuthentication{Type: types.ClientVpnAuthenticationType(awsapi.String(m, "Type"))}
		if mutual, ok := m["MutualAuthentication"].(map[string]any); ok {
			a.MutualAuthentication = &types.CertificateAuthentication{ClientRootCertificateChain: aws.String(awsapi.String(mutual, "ClientRootCertificateChainArn"))}
		}
		if federated, ok := m["FederatedAuthentication"].(map[string]any); ok {
			a.FederatedAuthentication = &types.FederatedAuthentication{SamlProviderArn: aws.String(awsapi.String(federated, "SAMLProviderArn"))}
		}
		if directory, ok := m["ActiveDirectory"].(map[string]any); ok {
			a.ActiveDirectory = &types.DirectoryServiceAuthentication{DirectoryId: aws.String(awsapi.String(directory, "DirectoryId"))}
		}
		e.AuthenticationOptions = append(e.AuthenticationOptions, a)
	}
	specs, _ := props["TagSpecifications"].([]any)
	for _, item := range specs {
		m, _ := item.(map[string]any)
		for _, t := range awsapi.Tags(m) {
			e.Tags = append(e.Tags, types.Tag{Key: aws.String(t.Key), Value: aws.String(t.Value)})
		}
	}
	e.Tags = append(e.Tags, types.Tag{Key: aws.String("aws:cloudformation:stack-name"), Value: aws.String("added-by-aws")})
	return e, nil
}

func (f *fakeAPI) DescribeClientVpnEndpoints(ctx context.Context, in *awsec2.DescribeClientVpnEndpointsInput, opts ...func(*awsec2.Options)) (*awsec2.DescribeClientVpnEndpointsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeClientVpnEndpoints", opts); err != nil {
		return nil, err
	}
	if err := validPage(in.MaxResults, len(in.ClientVpnEndpointIds), 5, 1000); err != nil {
		return nil, err
	}
	ids := in.ClientVpnEndpointIds
	if len(ids) == 0 {
		ids = f.store.List(clientVpnEndpointType)
	}
	out := &awsec2.DescribeClientVpnEndpointsOutput{}
	for _, id := range ids {
		props := f.store.Resource(clientVpnEndpointType, id)
		if props == nil {
			if len(in.ClientVpnEndpointIds) > 0 {
				return nil, fault("InvalidClientVpnEndpointId.NotFound")
			}
			continue
		}
		e, err := f.endpoint(id, props)
		if err != nil {
			return nil, err
		}
		out.ClientVpnEndpoints = append(out.ClientVpnEndpoints, e)
	}
	return out, nil
}

func (f *fakeAPI) ModifyClientVpnEndpoint(ctx context.Context, in *awsec2.ModifyClientVpnEndpointInput, opts ...func(*awsec2.Options)) (*awsec2.ModifyClientVpnEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyClientVpnEndpoint", opts); err != nil {
		return nil, err
	}
	var dnsServers []string
	if in.DnsServers != nil {
		dnsServers = in.DnsServers.CustomDnsServers
	}
	if err := validEndpointOptions(in.ConnectionLogOptions, in.ClientLoginBannerOptions, dnsServers, in.SelfServicePortal, in.SessionTimeoutHours, in.VpnPort); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClientVpnEndpointId)
	return &awsec2.ModifyClientVpnEndpointOutput{}, f.change(func() error {
		props := f.store.Resource(clientVpnEndpointType, id)
		if props == nil {
			return fault("InvalidClientVpnEndpointId.NotFound")
		}
		values, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		delete(values, "ClientVpnEndpointId")
		delete(values, "DnsServers")
		maps.Copy(props, values)
		if in.Description != nil && aws.ToString(in.Description) == "" {
			delete(props, "Description")
		}
		if dns := in.DnsServers; dns != nil {
			if aws.ToBool(dns.Enabled) {
				props["DnsServers"] = dns.CustomDnsServers
			} else {
				delete(props, "DnsServers")
			}
		}
		f.store.Put(clientVpnEndpointType, id, props)
		return nil
	})
}

func (f *fakeAPI) DeleteClientVpnEndpoint(ctx context.Context, in *awsec2.DeleteClientVpnEndpointInput, opts ...func(*awsec2.Options)) (*awsec2.DeleteClientVpnEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteClientVpnEndpoint", opts); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClientVpnEndpointId)
	return &awsec2.DeleteClientVpnEndpointOutput{}, f.change(func() error {
		if f.store.Resource(clientVpnEndpointType, id) == nil {
			return fault("InvalidClientVpnEndpointId.NotFound")
		}
		f.script[clientVpnEndpointType+"/"+id] = []string{"deleting", "deleted!"}
		return nil
	})
}

func (f *fakeAPI) GetClientVpnEndpointAuthorizationPolicy(ctx context.Context, in *awsec2.GetClientVpnEndpointAuthorizationPolicyInput, opts ...func(*awsec2.Options)) (*awsec2.GetClientVpnEndpointAuthorizationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetClientVpnEndpointAuthorizationPolicy", opts); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClientVpnEndpointId)
	props := f.store.Resource(clientVpnEndpointType, id)
	if props == nil {
		return nil, fault("InvalidClientVpnEndpointId.NotFound")
	}
	policy, ok := props["AuthorizationPolicy"].(map[string]any)
	if !ok {
		return nil, fault("InvalidClientVpnEndpointAuthorizationPolicy.NotFound")
	}
	out := &awsec2.GetClientVpnEndpointAuthorizationPolicyOutput{}
	if err := awsapi.Decode(policy, out); err != nil {
		return nil, err
	}
	out.ClientVpnEndpointId = aws.String(id)
	out.Status = types.ClientVpnAuthorizationPolicyStatus(f.status("policy", id, string(types.ClientVpnAuthorizationPolicyStatusActive)))
	return out, nil
}

func (f *fakeAPI) ModifyClientVpnEndpointAuthorizationPolicy(ctx context.Context, in *awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput, opts ...func(*awsec2.Options)) (*awsec2.ModifyClientVpnEndpointAuthorizationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ModifyClientVpnEndpointAuthorizationPolicy", opts); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClientVpnEndpointId)
	region := regionOf(opts)
	return &awsec2.ModifyClientVpnEndpointAuthorizationPolicyOutput{}, f.change(func() error {
		if err := oneOf(in.ShadowMode, types.ClientVpnAuthorizationPolicyShadowModeEnabled, types.ClientVpnAuthorizationPolicyShadowModeDisabled); err != nil {
			return err
		}
		props := f.store.Resource(clientVpnEndpointType, id)
		if props == nil {
			return fault("InvalidClientVpnEndpointId.NotFound")
		}
		use, earlier, err := f.useToken("ModifyClientVpnEndpointAuthorizationPolicy", region, in.ClientToken, in)
		if err != nil || earlier != nil {
			return err
		}
		values, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		delete(values, "ClientVpnEndpointId")
		delete(values, "ClientToken")
		// A call sets the values it has and keeps the others. A new policy needs a document, and is enforced unless
		// it says otherwise.
		policy, ok := props["AuthorizationPolicy"].(map[string]any)
		if !ok {
			if aws.ToString(in.PolicyDocument) == "" {
				return fault(missingParameter)
			}
			policy = map[string]any{"ShadowMode": string(types.ClientVpnAuthorizationPolicyShadowModeDisabled)}
		}
		maps.Copy(policy, values)
		if in.Description != nil && aws.ToString(in.Description) == "" {
			delete(policy, "Description")
		}
		props["AuthorizationPolicy"] = policy
		f.store.Put(clientVpnEndpointType, id, props)
		f.script["policy/"+id] = []string{string(types.ClientVpnAuthorizationPolicyStatusUpdating)}
		f.remember(use, id)
		return nil
	})
}

func (f *fakeAPI) DeleteClientVpnEndpointAuthorizationPolicy(ctx context.Context, in *awsec2.DeleteClientVpnEndpointAuthorizationPolicyInput, opts ...func(*awsec2.Options)) (*awsec2.DeleteClientVpnEndpointAuthorizationPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteClientVpnEndpointAuthorizationPolicy", opts); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ClientVpnEndpointId)
	return &awsec2.DeleteClientVpnEndpointAuthorizationPolicyOutput{}, f.change(func() error {
		props := f.store.Resource(clientVpnEndpointType, id)
		if props == nil {
			return fault("InvalidClientVpnEndpointId.NotFound")
		}
		delete(props, "AuthorizationPolicy")
		f.store.Put(clientVpnEndpointType, id, props)
		return nil
	})
}

func (f *fakeAPI) AssociateClientVpnTargetNetwork(ctx context.Context, in *awsec2.AssociateClientVpnTargetNetworkInput, opts ...func(*awsec2.Options)) (*awsec2.AssociateClientVpnTargetNetworkOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("AssociateClientVpnTargetNetwork", opts); err != nil {
		return nil, err
	}
	switch {
	case aws.ToString(in.SubnetId) == "" && aws.ToString(in.AvailabilityZone) == "" && aws.ToString(in.AvailabilityZoneId) == "":
		return nil, fault(missingParameter)
	case in.AvailabilityZone != nil && (in.SubnetId != nil || in.AvailabilityZoneId != nil):
		return nil, fault(invalidCombination)
	}
	endpoint := aws.ToString(in.ClientVpnEndpointId)
	if endpoint == missingEndpoint {
		return nil, fault("InvalidClientVpnEndpointId.NotFound")
	}
	use, earlier, err := f.useToken("AssociateClientVpnTargetNetwork", regionOf(opts), in.ClientToken, in)
	if err != nil {
		return nil, err
	}
	if earlier != nil && f.store.Resource(clientVpnTargetNetworkAssociationType, earlier.made) != nil {
		_, association, _ := strings.Cut(earlier.made, "|")
		return &awsec2.AssociateClientVpnTargetNetworkOutput{AssociationId: aws.String(association)}, nil
	}
	for _, key := range f.store.List(clientVpnTargetNetworkAssociationType) {
		props := f.store.Resource(clientVpnTargetNetworkAssociationType, key)
		if awsapi.String(props, "ClientVpnEndpointId") == endpoint && awsapi.String(props, "SubnetId") == aws.ToString(in.SubnetId) {
			return nil, fault("InvalidClientVpnDuplicateAssociationException")
		}
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	delete(props, "ClientToken")
	association := f.newID("cvpn-assoc-")
	props["AssociationId"] = association
	key := endpoint + "|" + association
	f.store.Put(clientVpnTargetNetworkAssociationType, key, props)
	f.script[clientVpnTargetNetworkAssociationType+"/"+key] = []string{string(types.AssociationStatusCodeAssociating)}
	f.remember(use, key)
	return &awsec2.AssociateClientVpnTargetNetworkOutput{AssociationId: aws.String(association)}, nil
}

func (f *fakeAPI) DescribeClientVpnTargetNetworks(ctx context.Context, in *awsec2.DescribeClientVpnTargetNetworksInput, opts ...func(*awsec2.Options)) (*awsec2.DescribeClientVpnTargetNetworksOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeClientVpnTargetNetworks", opts); err != nil {
		return nil, err
	}
	if err := validPage(in.MaxResults, len(in.AssociationIds), 5, 1000); err != nil {
		return nil, err
	}
	endpoint := aws.ToString(in.ClientVpnEndpointId)
	if endpoint == "" {
		return nil, fault(missingParameter)
	}
	if endpoint == missingEndpoint {
		return nil, fault("InvalidClientVpnEndpointId.NotFound")
	}
	out := &awsec2.DescribeClientVpnTargetNetworksOutput{}
	for _, association := range in.AssociationIds {
		key := endpoint + "|" + association
		props := f.store.Resource(clientVpnTargetNetworkAssociationType, key)
		if props == nil {
			return nil, fault("InvalidClientVpnAssociationIdNotFound")
		}
		status := f.status(clientVpnTargetNetworkAssociationType, key, string(types.AssociationStatusCodeAssociated))
		out.ClientVpnTargetNetworks = append(out.ClientVpnTargetNetworks, types.TargetNetwork{
			AssociationId:       aws.String(association),
			ClientVpnEndpointId: aws.String(endpoint),
			TargetNetworkId:     aws.String(awsapi.String(props, "SubnetId")),
			VpcId:               aws.String("vpc-1"),
			Status:              &types.AssociationStatus{Code: types.AssociationStatusCode(status)},
		})
	}
	return out, nil
}

func (f *fakeAPI) DisassociateClientVpnTargetNetwork(ctx context.Context, in *awsec2.DisassociateClientVpnTargetNetworkInput, opts ...func(*awsec2.Options)) (*awsec2.DisassociateClientVpnTargetNetworkOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DisassociateClientVpnTargetNetwork", opts); err != nil {
		return nil, err
	}
	key := aws.ToString(in.ClientVpnEndpointId) + "|" + aws.ToString(in.AssociationId)
	return &awsec2.DisassociateClientVpnTargetNetworkOutput{}, f.change(func() error {
		if f.store.Resource(clientVpnTargetNetworkAssociationType, key) == nil {
			return fault("InvalidClientVpnAssociationIdNotFound")
		}
		f.script[clientVpnTargetNetworkAssociationType+"/"+key] = []string{string(types.AssociationStatusCodeDisassociating), string(types.AssociationStatusCodeDisassociated) + "!"}
		return nil
	})
}

// entryProps returns the entry in the type's property format.
func entryProps(acl string, rule int32, egress bool, e types.NetworkAclEntry) map[string]any {
	key := entryKey{acl: acl, rule: rule, egress: egress}
	props := map[string]any{"Id": key.String(), "NetworkAclId": acl, "RuleNumber": rule, "Egress": egress, "RuleAction": string(e.RuleAction)}
	if n, err := strconv.Atoi(aws.ToString(e.Protocol)); err == nil {
		props["Protocol"] = n
	}
	if e.CidrBlock != nil {
		props["CidrBlock"] = aws.ToString(e.CidrBlock)
	}
	if e.Ipv6CidrBlock != nil {
		props["Ipv6CidrBlock"] = aws.ToString(e.Ipv6CidrBlock)
	}
	if r := e.PortRange; r != nil {
		props["PortRange"] = setNumbers(map[string]*int32{"From": r.From, "To": r.To})
	}
	if c := e.IcmpTypeCode; c != nil {
		props["Icmp"] = setNumbers(map[string]*int32{"Type": c.Type, "Code": c.Code})
	}
	return props
}

// setNumbers returns the values of numbers that are set, so one left out isn't kept as 0.
func setNumbers(numbers map[string]*int32) map[string]any {
	out := map[string]any{}
	for name, n := range numbers {
		if n != nil {
			out[name] = *n
		}
	}
	return out
}

// validEntryKey returns MissingParameter, as EC2 does, when a call leaves out the network ACL or the rule number, or
// Egress when the call needs it.
func validEntryKey(acl *string, rule *int32, egress *bool, needEgress bool) error {
	if acl == nil || rule == nil || (needEgress && egress == nil) {
		return fault(missingParameter)
	}
	return nil
}

// sdkEntry returns the entry the store has, in the API's format.
func sdkEntry(props map[string]any) (types.NetworkAclEntry, error) {
	e := types.NetworkAclEntry{}
	values := awsapi.Without(props, "Protocol", "Icmp")
	if icmp, ok := props["Icmp"]; ok {
		values["IcmpTypeCode"] = icmp
	}
	if err := awsapi.Decode(values, &e); err != nil {
		return e, err
	}
	e.Protocol = aws.String(fmt.Sprint(props["Protocol"]))
	e.Egress = aws.Bool(props["Egress"] == true)
	return e, nil
}

func (f *fakeAPI) aclExists(acl string) bool {
	if slices.Contains(f.acls, acl) {
		return true
	}
	for _, key := range f.store.List(networkAclEntryType) {
		if awsapi.String(f.store.Resource(networkAclEntryType, key), "NetworkAclId") == acl {
			return true
		}
	}
	return false
}

// validEntry returns the error EC2 returns for an entry CreateNetworkAclEntry or ReplaceNetworkAclEntry rejects.
func validEntry(e types.NetworkAclEntry) error {
	if aws.ToString(e.Protocol) == "" || e.RuleAction == "" || (e.CidrBlock == nil && e.Ipv6CidrBlock == nil) {
		return fault(missingParameter)
	}
	protocol, err := strconv.Atoi(aws.ToString(e.Protocol))
	numbered := err == nil
	if !slices.Contains([]types.RuleAction{types.RuleActionAllow, types.RuleActionDeny}, e.RuleAction) ||
		(numbered && (protocol < -1 || protocol > 255)) || !isBlock(e.CidrBlock, true) || !isBlock(e.Ipv6CidrBlock, false) {
		return fault(invalidValue)
	}
	if numbered && ((protocol == 6 || protocol == 17) && e.PortRange == nil ||
		(protocol == 1 || (protocol == 58 && e.Ipv6CidrBlock != nil)) && e.IcmpTypeCode == nil) {
		return fault(missingParameter)
	}
	return nil
}

// isBlock reports whether block is unset, or a CIDR block of IPv4 addresses when ipv4 is true, and of IPv6 addresses
// otherwise.
func isBlock(block *string, ipv4 bool) bool {
	if block == nil {
		return true
	}
	prefix, err := netip.ParsePrefix(*block)
	return err == nil && prefix.Addr().Is4() == ipv4
}

func (f *fakeAPI) CreateNetworkAclEntry(ctx context.Context, in *awsec2.CreateNetworkAclEntryInput, opts ...func(*awsec2.Options)) (*awsec2.CreateNetworkAclEntryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateNetworkAclEntry", opts); err != nil {
		return nil, err
	}
	if err := validEntryKey(in.NetworkAclId, in.RuleNumber, in.Egress, true); err != nil {
		return nil, err
	}
	acl, rule, egress := aws.ToString(in.NetworkAclId), aws.ToInt32(in.RuleNumber), aws.ToBool(in.Egress)
	key := entryKey{acl: acl, rule: rule, egress: egress}.String()
	entry := types.NetworkAclEntry{
		RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, Ipv6CidrBlock: in.Ipv6CidrBlock, PortRange: in.PortRange, IcmpTypeCode: in.IcmpTypeCode,
	}
	if err := validEntry(entry); err != nil {
		return nil, err
	}
	if rule < 1 || rule > maxRuleNumber {
		return nil, fault(invalidValue)
	}
	// AWS keeps an IPv4 block in its canonical form.
	if entry.CidrBlock != nil {
		entry.CidrBlock = aws.String(netip.MustParsePrefix(*entry.CidrBlock).Masked().String())
	}
	return &awsec2.CreateNetworkAclEntryOutput{}, f.change(func() error {
		if !f.aclExists(acl) {
			return fault("InvalidNetworkAclID.NotFound")
		}
		if f.store.Resource(networkAclEntryType, key) != nil {
			return fault("NetworkAclEntryAlreadyExists")
		}
		f.store.Put(networkAclEntryType, key, entryProps(acl, rule, egress, entry))
		return nil
	})
}

// ReplaceNetworkAclEntry replaces an entry. Egress defaults to false, the ingress rule, as with AWS.
func (f *fakeAPI) ReplaceNetworkAclEntry(ctx context.Context, in *awsec2.ReplaceNetworkAclEntryInput, opts ...func(*awsec2.Options)) (*awsec2.ReplaceNetworkAclEntryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ReplaceNetworkAclEntry", opts); err != nil {
		return nil, err
	}
	if err := validEntryKey(in.NetworkAclId, in.RuleNumber, in.Egress, false); err != nil {
		return nil, err
	}
	acl, rule, egress := aws.ToString(in.NetworkAclId), aws.ToInt32(in.RuleNumber), aws.ToBool(in.Egress)
	key := entryKey{acl: acl, rule: rule, egress: egress}.String()
	entry := types.NetworkAclEntry{
		RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, Ipv6CidrBlock: in.Ipv6CidrBlock, PortRange: in.PortRange, IcmpTypeCode: in.IcmpTypeCode,
	}
	if err := validEntry(entry); err != nil {
		return nil, err
	}
	return &awsec2.ReplaceNetworkAclEntryOutput{}, f.change(func() error {
		if f.store.Resource(networkAclEntryType, key) == nil {
			return fault("InvalidNetworkAclEntry.NotFound")
		}
		f.store.Put(networkAclEntryType, key, entryProps(acl, rule, egress, entry))
		return nil
	})
}

func (f *fakeAPI) DeleteNetworkAclEntry(ctx context.Context, in *awsec2.DeleteNetworkAclEntryInput, opts ...func(*awsec2.Options)) (*awsec2.DeleteNetworkAclEntryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteNetworkAclEntry", opts); err != nil {
		return nil, err
	}
	if err := validEntryKey(in.NetworkAclId, in.RuleNumber, in.Egress, true); err != nil {
		return nil, err
	}
	key := entryKey{acl: aws.ToString(in.NetworkAclId), rule: aws.ToInt32(in.RuleNumber), egress: aws.ToBool(in.Egress)}.String()
	return &awsec2.DeleteNetworkAclEntryOutput{}, f.change(func() error {
		if f.store.Resource(networkAclEntryType, key) == nil {
			return fault("InvalidNetworkAclEntry.NotFound")
		}
		f.store.Remove(networkAclEntryType, key)
		return nil
	})
}

// DescribeNetworkAcls returns the network ACLs with their entries, and the deny-all rules AWS adds to each.
func (f *fakeAPI) DescribeNetworkAcls(ctx context.Context, in *awsec2.DescribeNetworkAclsInput, opts ...func(*awsec2.Options)) (*awsec2.DescribeNetworkAclsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeNetworkAcls", opts); err != nil {
		return nil, err
	}
	if err := validPage(in.MaxResults, len(in.NetworkAclIds), 5, 1000); err != nil {
		return nil, err
	}
	acls := in.NetworkAclIds
	if len(acls) == 0 {
		acls = slices.Clone(f.acls)
		for _, key := range f.store.List(networkAclEntryType) {
			if acl := awsapi.String(f.store.Resource(networkAclEntryType, key), "NetworkAclId"); !slices.Contains(acls, acl) {
				acls = append(acls, acl)
			}
		}
	}
	out := &awsec2.DescribeNetworkAclsOutput{}
	for _, acl := range acls {
		if !f.aclExists(acl) {
			return nil, fault("InvalidNetworkAclID.NotFound")
		}
		item := types.NetworkAcl{NetworkAclId: aws.String(acl), VpcId: aws.String("vpc-1")}
		for _, key := range f.store.List(networkAclEntryType) {
			props := f.store.Resource(networkAclEntryType, key)
			if awsapi.String(props, "NetworkAclId") != acl {
				continue
			}
			e, err := sdkEntry(props)
			if err != nil {
				return nil, err
			}
			item.Entries = append(item.Entries, e)
		}
		for _, egress := range []bool{false, true} {
			item.Entries = append(item.Entries, types.NetworkAclEntry{
				RuleNumber: aws.Int32(32767), Egress: aws.Bool(egress), Protocol: aws.String("-1"), RuleAction: types.RuleActionDeny, CidrBlock: aws.String("0.0.0.0/0"),
			})
		}
		out.NetworkAcls = append(out.NetworkAcls, item)
	}
	return out, nil
}

func (f *fakeAPI) CreateNetworkInterfacePermission(ctx context.Context, in *awsec2.CreateNetworkInterfacePermissionInput, opts ...func(*awsec2.Options)) (*awsec2.CreateNetworkInterfacePermissionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateNetworkInterfacePermission", opts); err != nil {
		return nil, err
	}
	if in.Permission == "" {
		return nil, fault(missingParameter)
	}
	if err := oneOf(in.Permission, types.InterfacePermissionTypeInstanceAttach, types.InterfacePermissionTypeEipAssociate); err != nil {
		return nil, err
	}
	if aws.ToString(in.NetworkInterfaceId) == missingInterface {
		return nil, fault("InvalidNetworkInterfaceID.NotFound")
	}
	id := f.newID("eni-perm-")
	props := map[string]any{
		"Id":                 id,
		"NetworkInterfaceId": aws.ToString(in.NetworkInterfaceId),
		"AwsAccountId":       aws.ToString(in.AwsAccountId),
		"Permission":         string(in.Permission),
	}
	f.store.Put(networkInterfacePermissionType, id, props)
	f.script[networkInterfacePermissionType+"/"+id] = []string{string(types.NetworkInterfacePermissionStateCodePending)}
	p := f.permission(id, props)
	return &awsec2.CreateNetworkInterfacePermissionOutput{InterfacePermission: &p}, nil
}

// permission returns the permission the store has under id, with its state for a read.
func (f *fakeAPI) permission(id string, props map[string]any) types.NetworkInterfacePermission {
	state := f.status(networkInterfacePermissionType, id, string(types.NetworkInterfacePermissionStateCodeGranted))
	if state == string(types.NetworkInterfacePermissionStateCodeRevoked) {
		f.revoked = append(f.revoked, permissionOf(id, props, types.NetworkInterfacePermissionStateCodeRevoked))
	}
	return permissionOf(id, props, types.NetworkInterfacePermissionStateCode(state))
}

// permissionOf returns the permission the store has under id, in the API's format, with the state.
func permissionOf(id string, props map[string]any, state types.NetworkInterfacePermissionStateCode) types.NetworkInterfacePermission {
	return types.NetworkInterfacePermission{
		NetworkInterfacePermissionId: aws.String(id),
		NetworkInterfaceId:           aws.String(awsapi.String(props, "NetworkInterfaceId")),
		AwsAccountId:                 aws.String(awsapi.String(props, "AwsAccountId")),
		Permission:                   types.InterfacePermissionType(awsapi.String(props, "Permission")),
		PermissionState:              &types.NetworkInterfacePermissionState{State: state},
	}
}

// matches reports whether the permission matches every filter.
func matches(p types.NetworkInterfacePermission, filters []types.Filter) bool {
	for _, fl := range filters {
		var value string
		switch aws.ToString(fl.Name) {
		case "network-interface-permission.network-interface-id":
			value = aws.ToString(p.NetworkInterfaceId)
		case "network-interface-permission.aws-account-id":
			value = aws.ToString(p.AwsAccountId)
		case "network-interface-permission.permission":
			value = string(p.Permission)
		default:
			return false
		}
		if !slices.Contains(fl.Values, value) {
			return false
		}
	}
	return true
}

func (f *fakeAPI) DescribeNetworkInterfacePermissions(ctx context.Context, in *awsec2.DescribeNetworkInterfacePermissionsInput, opts ...func(*awsec2.Options)) (*awsec2.DescribeNetworkInterfacePermissionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeNetworkInterfacePermissions", opts); err != nil {
		return nil, err
	}
	if err := validPage(in.MaxResults, len(in.NetworkInterfacePermissionIds), 5, 255); err != nil {
		return nil, err
	}
	// Each permission that matches is read, which moves on its state, only when it is on the page.
	matching := []func() types.NetworkInterfacePermission{}
	for _, p := range f.revoked {
		if (len(in.NetworkInterfacePermissionIds) == 0 || slices.Contains(in.NetworkInterfacePermissionIds, aws.ToString(p.NetworkInterfacePermissionId))) && matches(p, in.Filters) {
			matching = append(matching, func() types.NetworkInterfacePermission { return p })
		}
	}
	for _, id := range f.store.List(networkInterfacePermissionType) {
		if len(in.NetworkInterfacePermissionIds) > 0 && !slices.Contains(in.NetworkInterfacePermissionIds, id) {
			continue
		}
		props := f.store.Resource(networkInterfacePermissionType, id)
		if matches(permissionOf(id, props, ""), in.Filters) {
			matching = append(matching, func() types.NetworkInterfacePermission { return f.permission(id, props) })
		}
	}
	if len(in.NetworkInterfacePermissionIds) > 0 && len(matching) == 0 {
		return nil, fault("InvalidPermissionId.NotFound")
	}
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	end := min(start+int(aws.ToInt32(cmp.Or(in.MaxResults, aws.Int32(permissionPageSize)))), len(matching))
	out := &awsec2.DescribeNetworkInterfacePermissionsOutput{}
	for _, read := range matching[min(start, end):end] {
		out.NetworkInterfacePermissions = append(out.NetworkInterfacePermissions, read())
	}
	if end < len(matching) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

// permissionPageSize is the most permissions DescribeNetworkInterfacePermissions returns without MaxResults.
const permissionPageSize = 50

func (f *fakeAPI) DeleteNetworkInterfacePermission(ctx context.Context, in *awsec2.DeleteNetworkInterfacePermissionInput, opts ...func(*awsec2.Options)) (*awsec2.DeleteNetworkInterfacePermissionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteNetworkInterfacePermission", opts); err != nil {
		return nil, err
	}
	id := aws.ToString(in.NetworkInterfacePermissionId)
	return &awsec2.DeleteNetworkInterfacePermissionOutput{}, f.change(func() error {
		if f.store.Resource(networkInterfacePermissionType, id) == nil {
			return fault("InvalidPermissionId.NotFound")
		}
		f.script[networkInterfacePermissionType+"/"+id] = []string{string(types.NetworkInterfacePermissionStateCodeRevoking), string(types.NetworkInterfacePermissionStateCodeRevoked) + "!"}
		return nil
	})
}

// propagationEntries returns the keys of the route propagation entries of the gateway, by store type.
func (f *fakeAPI) propagationEntries(gateway string) map[string][]string {
	out := map[string][]string{}
	for _, typeName := range []string{vpnGatewayRoutePropagationType, outsidePropagationType} {
		for _, key := range f.store.List(typeName) {
			if awsapi.String(f.store.Resource(typeName, key), "VpnGatewayId") == gateway {
				out[typeName] = append(out[typeName], key)
			}
		}
	}
	return out
}

// propagatesTo reports whether the gateway propagates to the route table.
func (f *fakeAPI) propagatesTo(gateway string, table string) bool {
	for typeName, keys := range f.propagationEntries(gateway) {
		for _, key := range keys {
			if slices.Contains(awsapi.Strings(f.store.Resource(typeName, key), "RouteTableIds"), table) {
				return true
			}
		}
	}
	return false
}

// validPropagation returns the error EC2 returns for a gateway or route table that doesn't exist. In the fake, the
// gateways are the IDs with the prefix vgw-, and the route tables the IDs with the prefix rtb-.
func validPropagation(gateway string, table string) error {
	if !strings.HasPrefix(gateway, "vgw-") {
		return fault("InvalidVpnGatewayID.NotFound")
	}
	if !strings.HasPrefix(table, "rtb-") {
		return fault("InvalidRouteTableID.NotFound")
	}
	return nil
}

func (f *fakeAPI) EnableVgwRoutePropagation(ctx context.Context, in *awsec2.EnableVgwRoutePropagationInput, opts ...func(*awsec2.Options)) (*awsec2.EnableVgwRoutePropagationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("EnableVgwRoutePropagation", opts); err != nil {
		return nil, err
	}
	gateway, table := aws.ToString(in.GatewayId), aws.ToString(in.RouteTableId)
	return &awsec2.EnableVgwRoutePropagationOutput{}, f.change(func() error {
		if err := validPropagation(gateway, table); err != nil {
			return err
		}
		if strings.HasPrefix(gateway, detachedGateway) {
			return fault("InvalidVpnGatewayAttachment.NotFound")
		}
		entry := f.store.Resource(vpnGatewayRoutePropagationType, f.current)
		if entry == nil {
			entry = map[string]any{"Id": f.current, "VpnGatewayId": gateway, "RouteTableIds": []any{}}
		}
		tables := awsapi.Strings(entry, "RouteTableIds")
		if !slices.Contains(tables, table) {
			tables = append(tables, table)
		}
		entry["RouteTableIds"] = tables
		f.store.Put(vpnGatewayRoutePropagationType, f.current, entry)
		return nil
	})
}

func (f *fakeAPI) DisableVgwRoutePropagation(ctx context.Context, in *awsec2.DisableVgwRoutePropagationInput, opts ...func(*awsec2.Options)) (*awsec2.DisableVgwRoutePropagationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DisableVgwRoutePropagation", opts); err != nil {
		return nil, err
	}
	gateway, table := aws.ToString(in.GatewayId), aws.ToString(in.RouteTableId)
	return &awsec2.DisableVgwRoutePropagationOutput{}, f.change(func() error {
		if err := validPropagation(gateway, table); err != nil {
			return err
		}
		for typeName, keys := range f.propagationEntries(gateway) {
			for _, key := range keys {
				entry := f.store.Resource(typeName, key)
				tables := slices.DeleteFunc(awsapi.Strings(entry, "RouteTableIds"), func(t string) bool { return t == table })
				if len(tables) == 0 {
					f.store.Remove(typeName, key)
					continue
				}
				entry["RouteTableIds"] = tables
				f.store.Put(typeName, key, entry)
			}
		}
		return nil
	})
}

// DescribeRouteTables returns the route tables the route-table-id filter names that exist, or every route table the
// store names and rtb-unrelated, with the gateways that propagate to each.
func (f *fakeAPI) DescribeRouteTables(ctx context.Context, in *awsec2.DescribeRouteTablesInput, opts ...func(*awsec2.Options)) (*awsec2.DescribeRouteTablesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeRouteTables", opts); err != nil {
		return nil, err
	}
	if err := validPage(in.MaxResults, len(in.RouteTableIds), 5, 100); err != nil {
		return nil, err
	}
	gateways := []string{}
	tables := []string{"rtb-unrelated"}
	for _, typeName := range []string{vpnGatewayRoutePropagationType, outsidePropagationType} {
		for _, key := range f.store.List(typeName) {
			entry := f.store.Resource(typeName, key)
			if gateway := awsapi.String(entry, "VpnGatewayId"); !slices.Contains(gateways, gateway) {
				gateways = append(gateways, gateway)
			}
			for _, table := range awsapi.Strings(entry, "RouteTableIds") {
				if !slices.Contains(tables, table) {
					tables = append(tables, table)
				}
			}
		}
	}
	for _, fl := range in.Filters {
		if aws.ToString(fl.Name) != "route-table-id" {
			return nil, fault("InvalidParameterValue")
		}
		tables = slices.DeleteFunc(slices.Clone(fl.Values), func(t string) bool { return !strings.HasPrefix(t, "rtb-") })
	}
	out := &awsec2.DescribeRouteTablesOutput{}
	for _, table := range tables {
		rt := types.RouteTable{RouteTableId: aws.String(table), VpcId: aws.String("vpc-1")}
		for _, gateway := range gateways {
			if f.propagatesTo(gateway, table) {
				rt.PropagatingVgws = append(rt.PropagatingVgws, types.PropagatingVgw{GatewayId: aws.String(gateway)})
			}
		}
		out.RouteTables = append(out.RouteTables, rt)
	}
	return out, nil
}
