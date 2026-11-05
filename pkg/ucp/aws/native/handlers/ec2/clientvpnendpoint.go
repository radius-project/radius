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
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const clientVpnEndpointType = "AWS::EC2::ClientVpnEndpoint"

var _ native.Handler = (*ClientVpnEndpoint)(nil)

// endpointStates are the states a create or update waits for. A new endpoint is pending-associate until a target
// network is associated with it, which another resource does.
var endpointStates = awsapi.States{Done: []string{string(types.ClientVpnEndpointStatusCodeAvailable), string(types.ClientVpnEndpointStatusCodePendingAssociate)}}

// ClientVpnEndpoint serves AWS::EC2::ClientVpnEndpoint. Its identifier, Id, is the endpoint ID, such as
// cvpn-endpoint-0123456789abcdef0, which AWS assigns. CreateClientVpnEndpoint takes the record's client token as its
// idempotency token, so a create sent again returns the first endpoint. The authorization policy, AuthorizationPolicy,
// is set with its own calls. Tags are create-only, set through TagSpecifications.
type ClientVpnEndpoint struct {
	api API
}

// NewClientVpnEndpoint returns the handler for Client VPN endpoints.
func NewClientVpnEndpoint(api API) *ClientVpnEndpoint {
	return &ClientVpnEndpoint{api: api}
}

// TypeName returns AWS::EC2::ClientVpnEndpoint.
func (h *ClientVpnEndpoint) TypeName() string {
	return clientVpnEndpointType
}

// Options returns how UCP uses the handler.
func (h *ClientVpnEndpoint) Options() native.Options {
	return native.Options{
		List:     native.ListRegion,
		Recovery: native.RecoverByResend,
	}
}

// describe returns the endpoint with the identifier, or ErrNotFound when AWS has none or it is deleted.
func (h *ClientVpnEndpoint) describe(ctx context.Context, region string, identifier string) (*types.ClientVpnEndpoint, error) {
	out, err := h.api.DescribeClientVpnEndpoints(ctx, &awsec2.DescribeClientVpnEndpointsInput{ClientVpnEndpointIds: []string{identifier}}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "InvalidClientVpnEndpointId.NotFound")
	}
	for _, e := range out.ClientVpnEndpoints {
		if aws.ToString(e.ClientVpnEndpointId) == identifier && !endpointDeleted(&e) {
			return &e, nil
		}
	}
	return nil, native.ErrNotFound
}

func endpointDeleted(e *types.ClientVpnEndpoint) bool {
	return e.Status != nil && e.Status.Code == types.ClientVpnEndpointStatusCodeDeleted
}

func endpointStatus(e *types.ClientVpnEndpoint) string {
	if e.Status == nil {
		return ""
	}
	return string(e.Status.Code)
}

// policy returns the endpoint's authorization policy, or nil when it has none.
func (h *ClientVpnEndpoint) policy(ctx context.Context, region string, identifier string) (*awsec2.GetClientVpnEndpointAuthorizationPolicyOutput, error) {
	out, err := h.api.GetClientVpnEndpointAuthorizationPolicy(ctx, &awsec2.GetClientVpnEndpointAuthorizationPolicyInput{ClientVpnEndpointId: aws.String(identifier)}, inRegion(region))
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && strings.Contains(apiErr.ErrorCode(), "NotFound") {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if aws.ToString(out.PolicyDocument) == "" {
		return nil, nil
	}
	return out, nil
}

// Read returns the endpoint's properties.
func (h *ClientVpnEndpoint) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, e)
}

// properties returns the endpoint's properties. AWS returns some values with other names than the template sets, such
// as ClientRootCertificateChain for ClientRootCertificateChainArn, and the self-service portal's URL instead of
// SelfServicePortal.
func (h *ClientVpnEndpoint) properties(ctx context.Context, region string, e *types.ClientVpnEndpoint) (map[string]any, error) {
	id := aws.ToString(e.ClientVpnEndpointId)
	policy, err := h.policy(ctx, region, id)
	if err != nil {
		return nil, err
	}
	p := map[string]any{
		"Id":                            id,
		"AuthenticationOptions":         authenticationOptions(e.AuthenticationOptions),
		"ClientCidrBlock":               e.ClientCidrBlock,
		"ClientLoginBannerOptions":      e.ClientLoginBannerOptions,
		"ClientRouteEnforcementOptions": e.ClientRouteEnforcementOptions,
		"ConnectionLogOptions":          e.ConnectionLogOptions,
		"Description":                   e.Description,
		"DisconnectOnSessionTimeout":    e.DisconnectOnSessionTimeout,
		"DnsServers":                    e.DnsServers,
		"EndpointIpAddressType":         string(e.EndpointIpAddressType),
		"SecurityGroupIds":              e.SecurityGroupIds,
		"SelfServicePortal":             string(types.SelfServicePortalDisabled),
		"ServerCertificateArn":          e.ServerCertificateArn,
		"SessionTimeoutHours":           e.SessionTimeoutHours,
		"SplitTunnel":                   e.SplitTunnel,
		"TrafficIpAddressType":          string(e.TrafficIpAddressType),
		"TransportProtocol":             string(e.TransportProtocol),
		"VpcId":                         e.VpcId,
		"VpnPort":                       e.VpnPort,
	}
	if aws.ToString(e.SelfServicePortalUrl) != "" {
		p["SelfServicePortal"] = string(types.SelfServicePortalEnabled)
	}
	if o := e.ClientConnectOptions; o != nil {
		p["ClientConnectOptions"] = map[string]any{"Enabled": o.Enabled, "LambdaFunctionArn": o.LambdaFunctionArn}
	}
	if o := e.DevicePostureOptions; o != nil {
		p["DevicePostureOptions"] = map[string]any{"TrustProviders": o.TrustProviders}
	}
	if o := e.TransitGatewayConfiguration; o != nil {
		p["TransitGatewayConfiguration"] = map[string]any{"TransitGatewayId": o.TransitGatewayId, "AvailabilityZones": o.AvailabilityZones, "AvailabilityZoneIds": o.AvailabilityZoneIds}
	}
	if tags := endpointTags(e.Tags); tags != nil {
		p["TagSpecifications"] = []any{map[string]any{"ResourceType": string(types.ResourceTypeClientVpnEndpoint), "Tags": tags}}
	}
	if policy != nil {
		p["AuthorizationPolicy"] = map[string]any{"PolicyDocument": policy.PolicyDocument, "ShadowMode": string(policy.ShadowMode), "Description": policy.Description}
	}
	return awsapi.EncodeObject(p)
}

// authenticationOptions returns the endpoint's authentication options with the names the template sets.
func authenticationOptions(options []types.ClientVpnAuthentication) []any {
	out := []any{}
	for _, o := range options {
		m := map[string]any{"Type": string(o.Type)}
		if a := o.ActiveDirectory; a != nil {
			m["ActiveDirectory"] = map[string]any{"DirectoryId": a.DirectoryId}
		}
		if a := o.MutualAuthentication; a != nil {
			m["MutualAuthentication"] = map[string]any{"ClientRootCertificateChainArn": a.ClientRootCertificateChain}
		}
		if a := o.FederatedAuthentication; a != nil {
			m["FederatedAuthentication"] = map[string]any{"SAMLProviderArn": a.SamlProviderArn, "SelfServiceSAMLProviderArn": a.SelfServiceSamlProviderArn}
		}
		out = append(out, m)
	}
	return out
}

func endpointTags(tags []types.Tag) []any {
	m := map[string]string{}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return awsapi.TagList(m)
}

// setPolicy sets the endpoint's authorization policy to policy, the AuthorizationPolicy property. token is the call's
// idempotency token, or nil for one the SDK makes.
func (h *ClientVpnEndpoint) setPolicy(ctx context.Context, region string, identifier string, policy any, token *string) error {
	input := &awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput{}
	values, _ := policy.(map[string]any)
	if err := awsapi.Decode(values, input); err != nil {
		return err
	}
	input.ClientVpnEndpointId = aws.String(identifier)
	input.ClientToken = token
	_, err := h.api.ModifyClientVpnEndpointAuthorizationPolicy(ctx, input, inRegion(region))
	return err
}

// Create creates the endpoint, then sets its authorization policy when the template has one. A create that fails
// after the endpoint exists returns the endpoint ID with the failure, so UCP saves it.
func (h *ClientVpnEndpoint) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	input := &awsec2.CreateClientVpnEndpointInput{}
	if err := awsapi.Decode(awsapi.Without(request.Properties, "AuthorizationPolicy"), input); err != nil {
		return "", err
	}
	input.ClientToken = aws.String(request.ClientToken)
	identifier := ""
	calls := []func() error{func() error {
		out, err := h.api.CreateClientVpnEndpoint(ctx, input, inRegion(request.Region))
		if err == nil {
			identifier = aws.ToString(out.ClientVpnEndpointId)
		}
		return err
	}}
	if policy, ok := request.Properties["AuthorizationPolicy"]; ok {
		calls = append(calls, func() error {
			return h.setPolicy(ctx, request.Region, identifier, policy, input.ClientToken)
		})
	}
	if err := awsapi.Calls(calls...); err != nil {
		var failure *native.Failure
		if errors.As(err, &failure) {
			return identifier, err
		}
		return "", err
	}
	return identifier, nil
}

// endpointModifiable are the properties ModifyClientVpnEndpoint changes. The authorization policy changes through its
// own calls.
var endpointModifiable = []string{
	"ClientConnectOptions", "ClientLoginBannerOptions", "ClientRouteEnforcementOptions", "ConnectionLogOptions",
	"Description", "DevicePostureOptions", "DisconnectOnSessionTimeout", "DnsServers", "SecurityGroupIds",
	"SelfServicePortal", "ServerCertificateArn", "SessionTimeoutHours", "SplitTunnel", "VpcId", "VpnPort",
}

// endpointDefaults are the values a property goes back to when the template removes it, which are AWS's defaults.
// AWS keeps the value of any other removed property.
var endpointDefaults = map[string]any{
	"ClientConnectOptions":          map[string]any{"Enabled": false},
	"ClientLoginBannerOptions":      map[string]any{"Enabled": false},
	"ClientRouteEnforcementOptions": map[string]any{"Enforced": false},
	"Description":                   "",
	"DevicePostureOptions":          map[string]any{"Enabled": false},
	"DisconnectOnSessionTimeout":    true,
	"DnsServers":                    []any{},
	"SelfServicePortal":             string(types.SelfServicePortalDisabled),
	"SessionTimeoutHours":           24,
	"SplitTunnel":                   false,
	"VpnPort":                       443,
}

// Update makes the patch's changes with ModifyClientVpnEndpoint, then sets or deletes the authorization policy.
func (h *ClientVpnEndpoint) Update(ctx context.Context, request native.UpdateRequest) error {
	changed := awsapi.Changed(request)
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(endpointModifiable, name) {
			continue
		}
		value, ok := request.Desired[name]
		if !ok {
			if value, ok = endpointDefaults[name]; !ok {
				continue
			}
		}
		values[name] = value
	}
	calls := []func() error{}
	if len(values) > 0 {
		input := &awsec2.ModifyClientVpnEndpointInput{}
		if err := awsapi.Decode(awsapi.Without(values, "DnsServers"), input); err != nil {
			return err
		}
		input.ClientVpnEndpointId = aws.String(request.Identifier)
		if _, ok := values["DnsServers"]; ok {
			servers := awsapi.Strings(values, "DnsServers")
			input.DnsServers = &types.DnsServersOptionsModifyStructure{CustomDnsServers: servers, Enabled: aws.Bool(len(servers) > 0)}
		}
		calls = append(calls, func() error {
			_, err := h.api.ModifyClientVpnEndpoint(ctx, input, inRegion(request.Region))
			return awsapi.NotFound(err, "InvalidClientVpnEndpointId.NotFound")
		})
	}
	if slices.Contains(changed, "AuthorizationPolicy") {
		if policy, ok := request.Desired["AuthorizationPolicy"]; ok {
			calls = append(calls, func() error {
				return h.setPolicy(ctx, request.Region, request.Identifier, policy, nil)
			})
		} else {
			calls = append(calls, func() error {
				_, err := h.api.DeleteClientVpnEndpointAuthorizationPolicy(ctx, &awsec2.DeleteClientVpnEndpointAuthorizationPolicyInput{ClientVpnEndpointId: aws.String(request.Identifier)}, inRegion(request.Region))
				return err
			})
		}
	}
	return awsapi.Calls(calls...)
}

// Delete deletes the endpoint. AWS rejects the delete while target networks are associated with it. An endpoint AWS
// is already deleting had a delete reach AWS before, so the delete succeeds.
func (h *ClientVpnEndpoint) Delete(ctx context.Context, region string, identifier string) error {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	if endpointStatus(e) == string(types.ClientVpnEndpointStatusCodeDeleting) {
		return nil
	}
	_, err = h.api.DeleteClientVpnEndpoint(ctx, &awsec2.DeleteClientVpnEndpointInput{ClientVpnEndpointId: aws.String(identifier)}, inRegion(region))
	return awsapi.NotFound(err, "InvalidClientVpnEndpointId.NotFound")
}

// List returns every Client VPN endpoint in the region that is not deleted.
func (h *ClientVpnEndpoint) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := awsec2.NewDescribeClientVpnEndpointsPaginator(h.api, &awsec2.DescribeClientVpnEndpointsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, e := range page.ClientVpnEndpoints {
			if endpointDeleted(&e) {
				continue
			}
			properties, err := h.properties(ctx, region, &e)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(e.ClientVpnEndpointId), Properties: properties})
		}
	}
	return out, nil
}

// Progress reads the endpoint's status. A create or update waits for the endpoint to be pending-associate or
// available, and for its authorization policy, when it has one, to be active. A delete waits for the endpoint to be
// gone.
func (h *ClientVpnEndpoint) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := endpointStatus(e)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: string(types.ClientVpnEndpointStatusCodeDeleted)}, nil
	}
	progress := endpointStates.Progress(status)
	if !progress.Done {
		return progress, nil
	}
	policy, err := h.policy(ctx, region, identifier)
	if err != nil || policy == nil {
		return progress, err
	}
	switch policy.Status {
	case types.ClientVpnAuthorizationPolicyStatusFailed:
		return native.Progress{Done: true, State: string(policy.Status), Failure: &native.Failure{Message: fmt.Sprintf("AWS reports the authorization policy of the Client VPN endpoint %q as failed", identifier)}}, nil
	case types.ClientVpnAuthorizationPolicyStatusCreating, types.ClientVpnAuthorizationPolicyStatusUpdating, types.ClientVpnAuthorizationPolicyStatusDeleting:
		return native.Progress{State: "authorization policy " + string(policy.Status), Waiting: string(types.ClientVpnAuthorizationPolicyStatusActive)}, nil
	}
	return progress, nil
}
