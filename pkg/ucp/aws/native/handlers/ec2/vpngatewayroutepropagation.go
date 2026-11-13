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

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const vpnGatewayRoutePropagationType = "AWS::EC2::VPNGatewayRoutePropagation"

var (
	_ native.Handler       = (*VPNGatewayRoutePropagation)(nil)
	_ native.ScopedReader  = (*VPNGatewayRoutePropagation)(nil)
	_ native.ScopedDeleter = (*VPNGatewayRoutePropagation)(nil)
)

// VPNGatewayRoutePropagation serves AWS::EC2::VPNGatewayRoutePropagation: route tables a template has a virtual
// private gateway propagate its routes to. Others can turn on propagation from the same gateway to other route
// tables, so the handler reads, changes, and deletes only the route tables the alias manages. Its identifier, Id, is
// VpnGatewayId and a name made from the record's client token, joined with "|". Turning propagation on for a route
// table that already has it changes nothing, so UCP finds a lost create by sending it again. Every call takes effect
// at once.
type VPNGatewayRoutePropagation struct {
	api API
}

// NewVPNGatewayRoutePropagation returns the handler for route propagation from virtual private gateways.
func NewVPNGatewayRoutePropagation(api API) *VPNGatewayRoutePropagation {
	return &VPNGatewayRoutePropagation{api: api}
}

// TypeName returns AWS::EC2::VPNGatewayRoutePropagation.
func (h *VPNGatewayRoutePropagation) TypeName() string {
	return vpnGatewayRoutePropagationType
}

// Options returns how UCP uses the handler.
func (h *VPNGatewayRoutePropagation) Options() native.Options {
	return native.Options{
		List:     native.ListNone,
		Recovery: native.RecoverByResend,
		Members:  &native.Members{Parent: "VpnGatewayId", List: "RouteTableIds"},
	}
}

// gatewayOf returns the gateway ID in identifier. An identifier without one, such as one another handler assigned,
// takes VpnGatewayId from the first of properties that sets it.
func gatewayOf(identifier string, properties ...map[string]any) (string, error) {
	if gateway, _, ok := strings.Cut(identifier, "|"); ok && gateway != "" {
		return gateway, nil
	}
	for _, p := range properties {
		if gateway := awsapi.String(p, "VpnGatewayId"); gateway != "" {
			return gateway, nil
		}
	}
	return "", invalidIdentifier(vpnGatewayRoutePropagationType, identifier, "VpnGatewayId|name")
}

// routeTables returns the RouteTableIds property, without repeats.
func routeTables(properties map[string]any) []string {
	out := []string{}
	for _, id := range awsapi.Strings(properties, "RouteTableIds") {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// propagating returns the IDs of the route tables the gateway propagates to, sorted. When ids is not nil, it looks
// only at those route tables.
func (h *VPNGatewayRoutePropagation) propagating(ctx context.Context, region string, gateway string, ids []string) ([]string, error) {
	input := &awsec2.DescribeRouteTablesInput{}
	if ids != nil {
		if len(ids) == 0 {
			return []string{}, nil
		}
		// A filter, unlike RouteTableIds, leaves out a route table that no longer exists instead of failing.
		input.Filters = []types.Filter{filter("route-table-id", ids...)}
	}
	out := []string{}
	pages := awsec2.NewDescribeRouteTablesPaginator(h.api, input)
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, table := range page.RouteTables {
			if slices.ContainsFunc(table.PropagatingVgws, func(v types.PropagatingVgw) bool { return aws.ToString(v.GatewayId) == gateway }) {
				out = append(out, aws.ToString(table.RouteTableId))
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

// Read returns every route table the gateway propagates to.
func (h *VPNGatewayRoutePropagation) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.ReadScoped(ctx, region, identifier, nil)
}

// ReadScoped returns the route tables the gateway propagates to that one of managed lists, or every one when managed
// is nil. It returns ErrNotFound when there are none.
func (h *VPNGatewayRoutePropagation) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	gateway, err := gatewayOf(identifier, managed...)
	if err != nil {
		return nil, err
	}
	var ids []string
	if managed != nil {
		ids = []string{}
		for _, properties := range managed {
			for _, id := range routeTables(properties) {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
	}
	tables, err := h.propagating(ctx, region, gateway, ids)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, native.ErrNotFound
	}
	list := []any{}
	for _, id := range tables {
		list = append(list, id)
	}
	return map[string]any{"Id": identifier, "VpnGatewayId": gateway, "RouteTableIds": list}, nil
}

// enable returns the call that turns on propagation from the gateway to one route table. AWS takes one per call.
func (h *VPNGatewayRoutePropagation) enable(ctx context.Context, region string, gateway string, table string) func() error {
	return func() error {
		_, err := h.api.EnableVgwRoutePropagation(ctx, &awsec2.EnableVgwRoutePropagationInput{GatewayId: aws.String(gateway), RouteTableId: aws.String(table)}, inRegion(region))
		return err
	}
}

// disable returns the call that turns off propagation from the gateway to one route table. AWS takes one per call.
func (h *VPNGatewayRoutePropagation) disable(ctx context.Context, region string, gateway string, table string) func() error {
	return func() error {
		_, err := h.api.DisableVgwRoutePropagation(ctx, &awsec2.DisableVgwRoutePropagationInput{GatewayId: aws.String(gateway), RouteTableId: aws.String(table)}, inRegion(region))
		return err
	}
}

// Create turns on propagation to each route table, one call each, and returns the identifier. A create that fails
// after the first call returns the identifier with the failure, so UCP saves it.
func (h *VPNGatewayRoutePropagation) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	gateway := awsapi.String(request.Properties, "VpnGatewayId")
	tables := routeTables(request.Properties)
	if gateway == "" || len(tables) == 0 {
		return "", &native.Failure{Message: "set VpnGatewayId and at least one route table in RouteTableIds", ClientError: true, Outcome: native.OutcomeRejected}
	}
	identifier := gateway + "|" + native.GenerateName(request.ClientToken)
	calls := []func() error{}
	for _, table := range tables {
		calls = append(calls, h.enable(ctx, request.Region, gateway, table))
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

// Update turns off propagation to the route tables Current has and Desired doesn't, then turns it on for the ones
// Desired has and Current doesn't, one call each. A failure while turning propagation off is reported as rejected,
// even when its response is lost, so the record keeps every route table it may still have to turn off.
func (h *VPNGatewayRoutePropagation) Update(ctx context.Context, request native.UpdateRequest) error {
	gateway, err := gatewayOf(request.Identifier, request.Desired, request.Current)
	if err != nil {
		return err
	}
	current, desired := routeTables(request.Current), routeTables(request.Desired)
	calls := []func() error{}
	for _, table := range current {
		if slices.Contains(desired, table) {
			continue
		}
		disable := h.disable(ctx, request.Region, gateway, table)
		calls = append(calls, func() error {
			if err := disable(); err != nil {
				return &native.Failure{Message: fmt.Sprintf("failed to turn off route propagation to a route table: %s", err), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
			}
			return nil
		})
	}
	for _, table := range desired {
		if !slices.Contains(current, table) {
			calls = append(calls, h.enable(ctx, request.Region, gateway, table))
		}
	}
	return awsapi.Calls(calls...)
}

// Delete is not used: UCP deletes with DeleteScoped, so propagation Radius doesn't manage stays on.
func (h *VPNGatewayRoutePropagation) Delete(ctx context.Context, region string, identifier string) error {
	return fmt.Errorf("the native handler for %s deletes only the route propagation Radius manages, with DeleteScoped", vpnGatewayRoutePropagationType)
}

// DeleteScoped turns off propagation to the route tables managed lists that still have it, one call each. It returns
// ErrNotFound when none have it.
func (h *VPNGatewayRoutePropagation) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	if managed == nil {
		// With no record of what the alias turned on, turn off nothing rather than every propagation from the gateway.
		managed = []map[string]any{}
	}
	current, err := h.ReadScoped(ctx, region, identifier, managed)
	if err != nil {
		return err
	}
	gateway := awsapi.String(current, "VpnGatewayId")
	calls := []func() error{}
	for _, table := range routeTables(current) {
		disable := h.disable(ctx, region, gateway, table)
		calls = append(calls, func() error {
			if err := disable(); err != nil && !awsapi.HasCode(err, "InvalidRouteTableID.NotFound") {
				return err
			}
			return nil
		})
	}
	return awsapi.Calls(calls...)
}

// List is not supported: UCP calls it only for types it can list (native.ListRegion).
func (h *VPNGatewayRoutePropagation) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", vpnGatewayRoutePropagationType)
}

// Progress reports every operation as finished, because turning propagation on and off takes effect at once.
func (h *VPNGatewayRoutePropagation) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
