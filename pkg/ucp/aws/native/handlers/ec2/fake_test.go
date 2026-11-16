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
	"slices"
	"strconv"
	"strings"
	"sync"

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
	tokens  map[string]string
	revoked []types.NetworkInterfacePermission
	calls   []string

	// acls are the network ACLs that exist, besides the ones that have entries in the store.
	acls []string

	// current is the identifier of the route propagation entry a call that turns propagation on adds to.
	current string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, script: map[string][]string{}, tokens: map[string]string{}, errs: map[string]error{}, acls: []string{"acl-1", "acl-2"}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// call records a call, and returns the error errs sets for it. A call without a region fails, as with AWS.
func (f *fakeAPI) call(name string, opts []func(*awsec2.Options)) error {
	f.calls = append(f.calls, name)
	options := awsec2.Options{}
	for _, fn := range opts {
		fn(&options)
	}
	if options.Region == "" {
		return fmt.Errorf("the call %s has no region", name)
	}
	return f.errs[name]
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

// created returns the resource an earlier create sent with the client token made, if it is still there.
func (f *fakeAPI) created(typeName string, token string) (string, bool) {
	id, ok := f.tokens[typeName+"/"+token]
	return id, ok && f.store.Resource(typeName, id) != nil
}

func (f *fakeAPI) CreateClientVpnEndpoint(ctx context.Context, in *awsec2.CreateClientVpnEndpointInput, opts ...func(*awsec2.Options)) (*awsec2.CreateClientVpnEndpointOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateClientVpnEndpoint", opts); err != nil {
		return nil, err
	}
	if id, ok := f.created(clientVpnEndpointType, aws.ToString(in.ClientToken)); ok {
		return &awsec2.CreateClientVpnEndpointOutput{ClientVpnEndpointId: aws.String(id)}, nil
	}
	props, err := awsapi.EncodeObject(in)
	if err != nil {
		return nil, err
	}
	delete(props, "ClientToken")
	id := f.newID("cvpn-endpoint-")
	props["Id"] = id
	f.store.Put(clientVpnEndpointType, id, props)
	f.script[clientVpnEndpointType+"/"+id] = []string{"pending"}
	f.tokens[clientVpnEndpointType+"/"+aws.ToString(in.ClientToken)] = id
	return &awsec2.CreateClientVpnEndpointOutput{ClientVpnEndpointId: aws.String(id)}, nil
}

// endpoint returns the endpoint the store has under id, with the defaults AWS sets, in the API's format.
func (f *fakeAPI) endpoint(id string, props map[string]any) (types.ClientVpnEndpoint, error) {
	e := types.ClientVpnEndpoint{
		VpnPort:             aws.Int32(443),
		SessionTimeoutHours: aws.Int32(24),
		SplitTunnel:         aws.Bool(false),
		TransportProtocol:   types.TransportProtocolUdp,
	}
	if err := awsapi.Decode(awsapi.Without(props, "AuthenticationOptions", "TagSpecifications"), &e); err != nil {
		return e, err
	}
	e.ClientVpnEndpointId = aws.String(id)
	e.Status = &types.ClientVpnEndpointStatus{Code: types.ClientVpnEndpointStatusCode(f.status(clientVpnEndpointType, id, string(types.ClientVpnEndpointStatusCodePendingAssociate)))}
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
	return &awsec2.ModifyClientVpnEndpointAuthorizationPolicyOutput{}, f.change(func() error {
		props := f.store.Resource(clientVpnEndpointType, id)
		if props == nil {
			return fault("InvalidClientVpnEndpointId.NotFound")
		}
		policy, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		delete(policy, "ClientVpnEndpointId")
		delete(policy, "ClientToken")
		props["AuthorizationPolicy"] = policy
		f.store.Put(clientVpnEndpointType, id, props)
		f.script["policy/"+id] = []string{string(types.ClientVpnAuthorizationPolicyStatusUpdating)}
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
	if key, ok := f.created(clientVpnTargetNetworkAssociationType, aws.ToString(in.ClientToken)); ok {
		_, association, _ := strings.Cut(key, "|")
		return &awsec2.AssociateClientVpnTargetNetworkOutput{AssociationId: aws.String(association)}, nil
	}
	endpoint := aws.ToString(in.ClientVpnEndpointId)
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
	f.tokens[clientVpnTargetNetworkAssociationType+"/"+aws.ToString(in.ClientToken)] = key
	return &awsec2.AssociateClientVpnTargetNetworkOutput{AssociationId: aws.String(association)}, nil
}

func (f *fakeAPI) DescribeClientVpnTargetNetworks(ctx context.Context, in *awsec2.DescribeClientVpnTargetNetworksInput, opts ...func(*awsec2.Options)) (*awsec2.DescribeClientVpnTargetNetworksOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeClientVpnTargetNetworks", opts); err != nil {
		return nil, err
	}
	endpoint := aws.ToString(in.ClientVpnEndpointId)
	if endpoint == "cvpn-endpoint-missing" {
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
		props["PortRange"] = map[string]any{"From": aws.ToInt32(r.From), "To": aws.ToInt32(r.To)}
	}
	if c := e.IcmpTypeCode; c != nil {
		props["Icmp"] = map[string]any{"Type": aws.ToInt32(c.Type), "Code": aws.ToInt32(c.Code)}
	}
	return props
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

func (f *fakeAPI) CreateNetworkAclEntry(ctx context.Context, in *awsec2.CreateNetworkAclEntryInput, opts ...func(*awsec2.Options)) (*awsec2.CreateNetworkAclEntryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateNetworkAclEntry", opts); err != nil {
		return nil, err
	}
	acl, rule, egress := aws.ToString(in.NetworkAclId), aws.ToInt32(in.RuleNumber), aws.ToBool(in.Egress)
	key := entryKey{acl: acl, rule: rule, egress: egress}.String()
	return &awsec2.CreateNetworkAclEntryOutput{}, f.change(func() error {
		if !f.aclExists(acl) {
			return fault("InvalidNetworkAclID.NotFound")
		}
		if f.store.Resource(networkAclEntryType, key) != nil {
			return fault("NetworkAclEntryAlreadyExists")
		}
		f.store.Put(networkAclEntryType, key, entryProps(acl, rule, egress, types.NetworkAclEntry{
			RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, Ipv6CidrBlock: in.Ipv6CidrBlock, PortRange: in.PortRange, IcmpTypeCode: in.IcmpTypeCode,
		}))
		return nil
	})
}

func (f *fakeAPI) ReplaceNetworkAclEntry(ctx context.Context, in *awsec2.ReplaceNetworkAclEntryInput, opts ...func(*awsec2.Options)) (*awsec2.ReplaceNetworkAclEntryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ReplaceNetworkAclEntry", opts); err != nil {
		return nil, err
	}
	acl, rule, egress := aws.ToString(in.NetworkAclId), aws.ToInt32(in.RuleNumber), aws.ToBool(in.Egress)
	key := entryKey{acl: acl, rule: rule, egress: egress}.String()
	return &awsec2.ReplaceNetworkAclEntryOutput{}, f.change(func() error {
		if f.store.Resource(networkAclEntryType, key) == nil {
			return fault("InvalidNetworkAclEntry.NotFound")
		}
		f.store.Put(networkAclEntryType, key, entryProps(acl, rule, egress, types.NetworkAclEntry{
			RuleAction: in.RuleAction, Protocol: in.Protocol, CidrBlock: in.CidrBlock, Ipv6CidrBlock: in.Ipv6CidrBlock, PortRange: in.PortRange, IcmpTypeCode: in.IcmpTypeCode,
		}))
		return nil
	})
}

func (f *fakeAPI) DeleteNetworkAclEntry(ctx context.Context, in *awsec2.DeleteNetworkAclEntryInput, opts ...func(*awsec2.Options)) (*awsec2.DeleteNetworkAclEntryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteNetworkAclEntry", opts); err != nil {
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

func (f *fakeAPI) permission(id string, props map[string]any) types.NetworkInterfacePermission {
	state := f.status(networkInterfacePermissionType, id, string(types.NetworkInterfacePermissionStateCodeGranted))
	if state == string(types.NetworkInterfacePermissionStateCodeRevoked) {
		f.revoked = append(f.revoked, types.NetworkInterfacePermission{
			NetworkInterfacePermissionId: aws.String(id),
			NetworkInterfaceId:           aws.String(awsapi.String(props, "NetworkInterfaceId")),
			AwsAccountId:                 aws.String(awsapi.String(props, "AwsAccountId")),
			Permission:                   types.InterfacePermissionType(awsapi.String(props, "Permission")),
			PermissionState:              &types.NetworkInterfacePermissionState{State: types.NetworkInterfacePermissionStateCodeRevoked},
		})
	}
	return types.NetworkInterfacePermission{
		NetworkInterfacePermissionId: aws.String(id),
		NetworkInterfaceId:           aws.String(awsapi.String(props, "NetworkInterfaceId")),
		AwsAccountId:                 aws.String(awsapi.String(props, "AwsAccountId")),
		Permission:                   types.InterfacePermissionType(awsapi.String(props, "Permission")),
		PermissionState:              &types.NetworkInterfacePermissionState{State: types.NetworkInterfacePermissionStateCode(state)},
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
	out := &awsec2.DescribeNetworkInterfacePermissionsOutput{}
	for _, p := range f.revoked {
		if (len(in.NetworkInterfacePermissionIds) == 0 || slices.Contains(in.NetworkInterfacePermissionIds, aws.ToString(p.NetworkInterfacePermissionId))) && matches(p, in.Filters) {
			out.NetworkInterfacePermissions = append(out.NetworkInterfacePermissions, p)
		}
	}
	for _, id := range f.store.List(networkInterfacePermissionType) {
		if len(in.NetworkInterfacePermissionIds) > 0 && !slices.Contains(in.NetworkInterfacePermissionIds, id) {
			continue
		}
		p := f.permission(id, f.store.Resource(networkInterfacePermissionType, id))
		if matches(p, in.Filters) {
			out.NetworkInterfacePermissions = append(out.NetworkInterfacePermissions, p)
		}
	}
	if len(in.NetworkInterfacePermissionIds) > 0 && len(out.NetworkInterfacePermissions) == 0 {
		return nil, fault("InvalidPermissionId.NotFound")
	}
	return out, nil
}

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

func (f *fakeAPI) EnableVgwRoutePropagation(ctx context.Context, in *awsec2.EnableVgwRoutePropagationInput, opts ...func(*awsec2.Options)) (*awsec2.EnableVgwRoutePropagationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("EnableVgwRoutePropagation", opts); err != nil {
		return nil, err
	}
	gateway, table := aws.ToString(in.GatewayId), aws.ToString(in.RouteTableId)
	return &awsec2.EnableVgwRoutePropagationOutput{}, f.change(func() error {
		if !strings.HasPrefix(gateway, "vgw-") {
			return fault("InvalidVpnGatewayID.NotFound")
		}
		if !strings.HasPrefix(table, "rtb-") {
			return fault("InvalidRouteTableID.NotFound")
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
