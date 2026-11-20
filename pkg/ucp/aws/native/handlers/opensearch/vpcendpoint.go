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

package opensearch

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const vpcEndpointType = "AWS::OpenSearchService::VpcEndpoint"

// endpointTimeout is how long UCP waits for each operation on an endpoint. AWS says a new endpoint is active within
// 2 to 5 minutes.
const endpointTimeout = 30 * time.Minute

var _ native.Handler = (*VpcEndpoint)(nil)

// endpointIDPattern is the documented pattern of a VPC endpoint ID.
var endpointIDPattern = regexp.MustCompile(`^aos-[a-zA-Z0-9]*$`)

var (
	createStates = awsapi.States{Done: []string{string(types.VpcEndpointStatusActive)}, Failed: []string{string(types.VpcEndpointStatusCreateFailed)}}
	updateStates = awsapi.States{Done: []string{string(types.VpcEndpointStatusActive)}, Failed: []string{string(types.VpcEndpointStatusUpdateFailed)}}
)

// VpcEndpoint serves AWS::OpenSearchService::VpcEndpoint: an OpenSearch Service-managed interface VPC endpoint to a
// VPC domain. Its identifier, VpcEndpointId, is the endpoint ID AWS assigns. CreateVpcEndpoint takes the record's
// client token as its idempotency token, so a create sent again returns the first endpoint. UpdateVpcEndpoint
// changes the subnets and security groups, and the domain is create-only.
type VpcEndpoint struct {
	api API
	ec2 EC2API
}

// NewVpcEndpoint returns the handler for OpenSearch Service VPC endpoints. ec2 finds a VPC's default security group,
// which an update sends when the template removes SecurityGroupIds.
func NewVpcEndpoint(api API, ec2 EC2API) *VpcEndpoint {
	return &VpcEndpoint{api: api, ec2: ec2}
}

// TypeName returns AWS::OpenSearchService::VpcEndpoint.
func (h *VpcEndpoint) TypeName() string {
	return vpcEndpointType
}

// Options returns how UCP uses the handler.
func (h *VpcEndpoint) Options() native.Options {
	return native.Options{
		List:          native.ListRegion,
		Recovery:      native.RecoverByResend,
		CreateTimeout: endpointTimeout,
		UpdateTimeout: endpointTimeout,
		DeleteTimeout: endpointTimeout,
	}
}

// checkEndpointID returns a client failure for an identifier that isn't a VPC endpoint ID, so it is never sent.
func checkEndpointID(identifier string) error {
	if len(identifier) < 5 || len(identifier) > 256 || !endpointIDPattern.MatchString(identifier) {
		return invalid(vpcEndpointType, "%q is not the ID of an OpenSearch Service VPC endpoint, such as aos-0123456789abcdef", identifier)
	}
	return nil
}

// describe returns the endpoint with the identifier, or ErrNotFound when AWS has none. DescribeVpcEndpoints reports a
// missing endpoint in VpcEndpointErrors, not as an error.
func (h *VpcEndpoint) describe(ctx context.Context, region string, identifier string) (*types.VpcEndpoint, error) {
	if err := checkEndpointID(identifier); err != nil {
		return nil, err
	}
	out, err := h.api.DescribeVpcEndpoints(ctx, &opensearch.DescribeVpcEndpointsInput{VpcEndpointIds: []string{identifier}}, inRegion(region))
	if err != nil {
		return nil, err
	}
	for i := range out.VpcEndpoints {
		if aws.ToString(out.VpcEndpoints[i].VpcEndpointId) == identifier {
			return &out.VpcEndpoints[i], nil
		}
	}
	for _, e := range out.VpcEndpointErrors {
		if aws.ToString(e.VpcEndpointId) != identifier {
			continue
		}
		if e.ErrorCode == types.VpcEndpointErrorCodeEndpointNotFound {
			return nil, native.ErrNotFound
		}
		return nil, fmt.Errorf("AWS failed to describe the OpenSearch Service VPC endpoint %q (%s): %s", identifier, e.ErrorCode, aws.ToString(e.ErrorMessage))
	}
	// AWS reports every endpoint it was asked for, so an answer that names neither can't be read as not found.
	return nil, fmt.Errorf("AWS returned nothing for the OpenSearch Service VPC endpoint %q", identifier)
}

// endpointProperties returns the endpoint's properties.
func endpointProperties(e *types.VpcEndpoint) (map[string]any, error) {
	p := map[string]any{
		"VpcEndpointId":    e.VpcEndpointId,
		"DomainArn":        e.DomainArn,
		"VpcEndpointOwner": e.VpcEndpointOwner,
		"Endpoint":         e.Endpoint,
		"Status":           string(e.Status),
	}
	if o := e.VpcOptions; o != nil {
		p["VpcOptions"] = map[string]any{
			"SubnetIds":         o.SubnetIds,
			"SecurityGroupIds":  o.SecurityGroupIds,
			"VPCId":             o.VPCId,
			"AvailabilityZones": o.AvailabilityZones,
		}
	}
	return awsapi.EncodeObject(p)
}

// Read returns the endpoint's properties.
func (h *VpcEndpoint) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return endpointProperties(e)
}

// vpcOptions returns the template's VpcOptions object and its subnets and security groups, nil when absent.
func vpcOptions(properties map[string]any) (options map[string]any, subnets []string, groups []string, err error) {
	if value := properties["VpcOptions"]; value != nil {
		var ok bool
		if options, ok = value.(map[string]any); !ok {
			return nil, nil, nil, invalid(vpcEndpointType, "VpcOptions must be an object with SubnetIds and SecurityGroupIds")
		}
	}
	subnets, ok := stringList(options, "SubnetIds")
	if !ok {
		return nil, nil, nil, invalid(vpcEndpointType, "VpcOptions.SubnetIds must be a list of subnet IDs")
	}
	groups, ok = stringList(options, "SecurityGroupIds")
	if !ok {
		return nil, nil, nil, invalid(vpcEndpointType, "VpcOptions.SecurityGroupIds must be a list of security group IDs")
	}
	// An empty list is never what AWS reports: an endpoint needs a subnet, and one created without security groups
	// gets the VPC's default group. So it is rejected, rather than sent on every deploy.
	if subnets != nil && len(subnets) == 0 {
		return nil, nil, nil, invalid(vpcEndpointType, "VpcOptions.SubnetIds needs at least one subnet")
	}
	if groups != nil && len(groups) == 0 {
		return nil, nil, nil, invalid(vpcEndpointType, "VpcOptions.SecurityGroupIds needs at least one security group. Leave it out to use the VPC's default security group")
	}
	return options, subnets, groups, nil
}

// busy returns the conflict of a change AWS can't make now because it is changing the endpoint, another endpoint of
// the domain, or the domain itself. UCP answers 409 and sends nothing more, so the caller tries again later.
func busy(what string, cause error) error {
	message := fmt.Sprintf("AWS is changing %s, so nothing was changed. Try again once the domain and its VPC endpoints are active, "+
		"and give VPC endpoints of one domain dependsOn on each other and on the domain", what)
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &native.Failure{Message: message, ClientError: true, Conflict: true, Outcome: native.OutcomeRejected}
}

// startError returns the error of a call that starts a change: a conflict for AWS's ConflictException, which AWS
// returns while it is changing the endpoint or its domain, and AWS's error otherwise, as a rejected client failure
// when AWS answered with a 4xx status.
func startError(err error, what string) error {
	if awsapi.HasCode(err, "ConflictException") {
		return busy(what, err)
	}
	return awsapi.Rejected(awsapi.NotFound(err, "ResourceNotFoundException"))
}

// Create starts creating the endpoint, sending the record's client token, so a create sent again returns the first
// endpoint instead of making a second one.
func (h *VpcEndpoint) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	domain, ok := request.Properties["DomainArn"].(string)
	if !ok || domain == "" {
		return "", invalid(vpcEndpointType, "DomainArn must be the ARN of an OpenSearch Service domain")
	}
	_, subnets, groups, err := vpcOptions(request.Properties)
	if err != nil {
		return "", err
	}
	out, err := h.api.CreateVpcEndpoint(ctx, &opensearch.CreateVpcEndpointInput{
		DomainArn:   aws.String(domain),
		VpcOptions:  &types.VPCOptions{SubnetIds: subnets, SecurityGroupIds: groups},
		ClientToken: aws.String(request.ClientToken),
	}, inRegion(request.Region))
	if err != nil {
		return "", startError(err, "the domain or another of its VPC endpoints")
	}
	if out.VpcEndpoint == nil || aws.ToString(out.VpcEndpoint.VpcEndpointId) == "" {
		return "", fmt.Errorf("AWS created an OpenSearch Service VPC endpoint for %q but returned no ID", domain)
	}
	return aws.ToString(out.VpcEndpoint.VpcEndpointId), nil
}

// changing reports whether AWS is still creating, updating, or deleting the endpoint.
func changing(status types.VpcEndpointStatus) bool {
	return status == types.VpcEndpointStatusCreating || status == types.VpcEndpointStatusUpdating || status == types.VpcEndpointStatusDeleting
}

// touches reports whether the patch changes path or a value inside it, or replaces an object that holds it.
func touches(request native.UpdateRequest, path string) bool {
	for _, op := range request.Patch {
		if op.Path == path || strings.HasPrefix(op.Path, path+"/") || strings.HasPrefix(path, op.Path+"/") || op.Path == "" {
			return true
		}
	}
	return false
}

// Update starts changing the endpoint's subnets and security groups. UpdateVpcEndpoint sets both, so the handler
// sends both: the template's, AWS's current value for one the template never set, and, for security groups the
// template removed, the VPC's default security group, which AWS uses when a create leaves them out. Subnets the
// template removed are sent as an empty list, which AWS rejects.
func (h *VpcEndpoint) Update(ctx context.Context, request native.UpdateRequest) error {
	_, subnets, groups, err := vpcOptions(request.Desired)
	if err != nil {
		return err
	}
	e, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return beforeChange(err)
	}
	if changing(e.Status) {
		return busy(fmt.Sprintf("the VPC endpoint %q (%s)", request.Identifier, e.Status), nil)
	}
	current := e.VpcOptions
	if current == nil {
		current = &types.VPCDerivedInfo{}
	}
	if subnets == nil {
		subnets = []string{}
		if !touches(request, "/VpcOptions/SubnetIds") {
			subnets = current.SubnetIds
		}
	}
	if groups == nil {
		if touches(request, "/VpcOptions/SecurityGroupIds") {
			if groups, err = h.defaultSecurityGroup(ctx, request.Region, aws.ToString(current.VPCId)); err != nil {
				return beforeChange(err)
			}
		} else {
			groups = current.SecurityGroupIds
		}
	}
	_, err = h.api.UpdateVpcEndpoint(ctx, &opensearch.UpdateVpcEndpointInput{
		VpcEndpointId: aws.String(request.Identifier),
		VpcOptions:    &types.VPCOptions{SubnetIds: subnets, SecurityGroupIds: groups},
	}, inRegion(request.Region))
	if err != nil {
		return startError(err, fmt.Sprintf("the VPC endpoint %q or its domain", request.Identifier))
	}
	return nil
}

// defaultSecurityGroup returns the default security group of the VPC, which every VPC has and can't delete.
func (h *VpcEndpoint) defaultSecurityGroup(ctx context.Context, region string, vpc string) ([]string, error) {
	if vpc == "" {
		return nil, fmt.Errorf("AWS reports no VPC for the endpoint, so Radius can't find the default security group to put back")
	}
	out, err := h.ec2.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{Filters: []ec2types.Filter{
		{Name: aws.String("vpc-id"), Values: []string{vpc}},
		{Name: aws.String("group-name"), Values: []string{"default"}},
	}}, ec2InRegion(region))
	if err != nil {
		return nil, err
	}
	if len(out.SecurityGroups) != 1 {
		return nil, fmt.Errorf("AWS reports %d default security groups for the VPC %q, so Radius can't tell which one to put back", len(out.SecurityGroups), vpc)
	}
	return []string{aws.ToString(out.SecurityGroups[0].GroupId)}, nil
}

// Delete starts deleting the endpoint. An endpoint AWS is already deleting had a delete reach AWS before, so the
// delete succeeds. An endpoint AWS is still creating or updating gets a conflict, and nothing is sent.
func (h *VpcEndpoint) Delete(ctx context.Context, region string, identifier string) error {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return beforeChange(err)
	}
	switch e.Status {
	case types.VpcEndpointStatusDeleting:
		return nil
	case types.VpcEndpointStatusCreating, types.VpcEndpointStatusUpdating:
		return busy(fmt.Sprintf("the VPC endpoint %q (%s)", identifier, e.Status), nil)
	}
	_, err = h.api.DeleteVpcEndpoint(ctx, &opensearch.DeleteVpcEndpointInput{VpcEndpointId: aws.String(identifier)}, inRegion(region))
	return startError(err, fmt.Sprintf("the VPC endpoint %q or its domain", identifier))
}

// List returns every endpoint in the region except those AWS is deleting.
func (h *VpcEndpoint) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	input := &opensearch.ListVpcEndpointsInput{}
	for {
		page, err := h.api.ListVpcEndpoints(ctx, input, inRegion(region))
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, s := range page.VpcEndpointSummaryList {
			if s.Status != types.VpcEndpointStatusDeleting {
				ids = append(ids, aws.ToString(s.VpcEndpointId))
			}
		}
		if len(ids) > 0 {
			described, err := h.api.DescribeVpcEndpoints(ctx, &opensearch.DescribeVpcEndpointsInput{VpcEndpointIds: ids}, inRegion(region))
			if err != nil {
				return nil, err
			}
			// An endpoint deleted since the list is reported not found, and any other error would leave it out.
			for _, e := range described.VpcEndpointErrors {
				if e.ErrorCode != types.VpcEndpointErrorCodeEndpointNotFound {
					return nil, fmt.Errorf("AWS failed to describe the OpenSearch Service VPC endpoint %q (%s): %s", aws.ToString(e.VpcEndpointId), e.ErrorCode, aws.ToString(e.ErrorMessage))
				}
			}
			for i := range described.VpcEndpoints {
				e := &described.VpcEndpoints[i]
				if e.Status == types.VpcEndpointStatusDeleting {
					continue
				}
				properties, err := endpointProperties(e)
				if err != nil {
					return nil, err
				}
				out = append(out, native.Resource{Identifier: aws.ToString(e.VpcEndpointId), Properties: properties})
			}
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return out, nil
		}
	}
}

// Progress reads the endpoint's status. A create or update waits for ACTIVE and fails at CREATE_FAILED or
// UPDATE_FAILED, which leave the endpoint in AWS, so the record stays and a later delete removes it. A delete waits
// for the endpoint to be gone, and fails at DELETE_FAILED.
func (h *VpcEndpoint) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := string(e.Status)
	switch op {
	case native.OperationCreate:
		return endpointProgress(createStates.Progress(status), identifier, "create"), nil
	case native.OperationUpdate:
		return endpointProgress(updateStates.Progress(status), identifier, "update"), nil
	}
	if e.Status == types.VpcEndpointStatusDeleteFailed {
		return native.Progress{Done: true, State: status, Failure: &native.Failure{
			Message: fmt.Sprintf("AWS failed to delete the OpenSearch Service VPC endpoint %q. Delete again to retry", identifier),
		}}, nil
	}
	return native.Progress{State: status, Waiting: "deleted"}, nil
}

// endpointProgress names the endpoint in the failure of a create or update, and what to do next. AWS keeps an
// endpoint it failed to create, and a create sent again with the record's client token returns that endpoint, so the
// resource must be deleted before a deploy can create it again. A failed update is sent again by the next deploy.
func endpointProgress(p native.Progress, identifier string, what string) native.Progress {
	if p.Failure == nil {
		return p
	}
	next := "Then deploy again"
	if what == "create" {
		next = "Delete the resource first, which deletes the failed endpoint, then deploy again"
	}
	p.Failure.Message = fmt.Sprintf("AWS failed to %s the OpenSearch Service VPC endpoint %q (%s). Check the subnets, security groups, and the domain's access. %s",
		what, identifier, p.State, next)
	return p
}
