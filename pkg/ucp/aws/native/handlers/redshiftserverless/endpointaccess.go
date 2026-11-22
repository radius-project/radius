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

package redshiftserverless

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	endpointAccessType = "AWS::RedshiftServerless::EndpointAccess"

	// endpointActive is the status of an endpoint that can be used and changed, and endpointDeleting the status of one
	// AWS is deleting.
	endpointActive   = "ACTIVE"
	endpointDeleting = "DELETING"

	maxEndpointName = 30
)

var (
	_ native.Handler           = (*EndpointAccess)(nil)
	_ native.IdentifierEncoder = (*EndpointAccess)(nil)

	// endpointName is the form of an endpoint name AWS takes. AWS also rejects two hyphens in a row and a hyphen at
	// the end, which validateEndpoint checks.
	endpointName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

	// ownerAccount is the form of an account ID.
	ownerAccount = regexp.MustCompile(`^[0-9]{12}$`)

	endpointStates = awsapi.States{Done: []string{endpointActive}}
)

// EndpointAccess serves AWS::RedshiftServerless::EndpointAccess. Its identifier is EndpointName, which UCP generates
// when the template leaves it out.
type EndpointAccess struct {
	api API
}

// NewEndpointAccess returns the handler for Redshift Serverless managed VPC endpoints.
func NewEndpointAccess(api API) *EndpointAccess {
	return &EndpointAccess{api: api}
}

// TypeName returns AWS::RedshiftServerless::EndpointAccess.
func (h *EndpointAccess) TypeName() string {
	return endpointAccessType
}

// Options returns how UCP uses the handler. A change of security groups makes the endpoint MODIFYING for minutes,
// so a change UCP doesn't need is not sent.
func (h *EndpointAccess) Options() native.Options {
	return native.Options{
		List:         native.ListRegion,
		Recovery:     native.RecoverByIdentifier,
		NameProperty: "EndpointName",
	}
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// EndpointName.
func (h *EndpointAccess) Identifier(properties map[string]any) (string, error) {
	if err := validateEndpoint(properties); err != nil {
		return "", err
	}
	return awsapi.String(properties, "EndpointName"), nil
}

// validateEndpoint checks that each value of the template has its schema's type and is one AWS takes.
func validateEndpoint(properties map[string]any) error {
	if err := checkStrings(endpointAccessType, properties, "EndpointName", "WorkgroupName", "OwnerAccount"); err != nil {
		return err
	}
	if name := awsapi.String(properties, "EndpointName"); name != "" &&
		(len(name) > maxEndpointName || !endpointName.MatchString(name) || strings.Contains(name, "--") || strings.HasSuffix(name, "-")) {
		return invalid(endpointAccessType, "EndpointName", "1 to 30 lowercase letters, digits, and hyphens, starting with a letter, without two hyphens in a row or a hyphen at the end")
	}
	if owner := awsapi.String(properties, "OwnerAccount"); owner != "" && !ownerAccount.MatchString(owner) {
		return invalid(endpointAccessType, "OwnerAccount", "a 12-digit account ID")
	}
	for _, name := range []string{"SubnetIds", "VpcSecurityGroupIds"} {
		if _, err := stringList(endpointAccessType, properties, name); err != nil {
			return err
		}
	}
	return nil
}

// securityGroups returns the IDs of the endpoint's security groups that AWS hasn't started removing.
func securityGroups(e *types.EndpointAccess) []string {
	out := []string{}
	for _, sg := range e.VpcSecurityGroups {
		if !strings.EqualFold(aws.ToString(sg.Status), "deleting") {
			out = append(out, aws.ToString(sg.VpcSecurityGroupId))
		}
	}
	return out
}

// endpointProperties returns the properties of an endpoint as AWS returns it.
func endpointProperties(e *types.EndpointAccess) map[string]any {
	properties := map[string]any{
		"SubnetIds":           anyList(e.SubnetIds),
		"VpcSecurityGroupIds": anyList(securityGroups(e)),
	}
	for name, value := range map[string]string{
		"EndpointName":   aws.ToString(e.EndpointName),
		"WorkgroupName":  aws.ToString(e.WorkgroupName),
		"EndpointArn":    aws.ToString(e.EndpointArn),
		"EndpointStatus": aws.ToString(e.EndpointStatus),
		"Address":        aws.ToString(e.Address),
	} {
		if value != "" {
			properties[name] = value
		}
	}
	if e.Port != nil {
		properties["Port"] = int64(*e.Port)
	}
	if v := e.VpcEndpoint; v != nil {
		endpoint := map[string]any{}
		if id := aws.ToString(v.VpcEndpointId); id != "" {
			endpoint["VpcEndpointId"] = id
		}
		if id := aws.ToString(v.VpcId); id != "" {
			endpoint["VpcId"] = id
		}
		interfaces := []any{}
		for _, ni := range v.NetworkInterfaces {
			item := map[string]any{}
			for name, value := range map[string]string{
				"AvailabilityZone":   aws.ToString(ni.AvailabilityZone),
				"Ipv6Address":        aws.ToString(ni.Ipv6Address),
				"NetworkInterfaceId": aws.ToString(ni.NetworkInterfaceId),
				"PrivateIpAddress":   aws.ToString(ni.PrivateIpAddress),
				"SubnetId":           aws.ToString(ni.SubnetId),
			} {
				if value != "" {
					item[name] = value
				}
			}
			interfaces = append(interfaces, item)
		}
		if len(interfaces) > 0 {
			endpoint["NetworkInterfaces"] = interfaces
		}
		if len(endpoint) > 0 {
			properties["VpcEndpoint"] = endpoint
		}
	}
	return properties
}

// get returns the endpoint with the name, or ErrNotFound.
func (h *EndpointAccess) get(ctx context.Context, region string, name string) (*types.EndpointAccess, error) {
	out, err := h.api.GetEndpointAccess(ctx, &redshiftserverless.GetEndpointAccessInput{EndpointName: aws.String(name)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFound)
	}
	if out.Endpoint == nil {
		return nil, native.ErrNotFound
	}
	return out.Endpoint, nil
}

// Read returns the endpoint's properties.
func (h *EndpointAccess) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	e, err := h.get(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return endpointProperties(e), nil
}

// Create creates the endpoint. The input reads each property by its exact name, so a key in another case can't send
// another value than the one the checks read.
func (h *EndpointAccess) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	if err := validateEndpoint(p); err != nil {
		return "", err
	}
	subnets, _ := stringList(endpointAccessType, p, "SubnetIds")
	groups, _ := stringList(endpointAccessType, p, "VpcSecurityGroupIds")
	name := awsapi.String(p, "EndpointName")
	input := &redshiftserverless.CreateEndpointAccessInput{
		EndpointName:        aws.String(name),
		WorkgroupName:       aws.String(awsapi.String(p, "WorkgroupName")),
		SubnetIds:           subnets,
		VpcSecurityGroupIds: groups,
	}
	if owner := awsapi.String(p, "OwnerAccount"); owner != "" {
		input.OwnerAccount = aws.String(owner)
	}
	_, err := h.api.CreateEndpointAccess(ctx, input, inRegion(request.Region))
	if err != nil {
		return created(name, endpointCreateError(awsapi.String(p, "WorkgroupName"), err))
	}
	return name, nil
}

// endpointCreateError returns the error of a create AWS rejected. AWS answers a name another endpoint has with
// ValidationException ("already exists"). UCP reads AWS by the name before it creates an endpoint, so that endpoint
// may be the one an earlier attempt of this create made, such as when the SDK retried a call whose response was lost.
// The outcome is then unknown, so UCP keeps the record and the next deploy finds the endpoint by its name.
func endpointCreateError(workgroup string, err error) error {
	if awsapi.HasCode(err, validation) && strings.Contains(err.Error(), "already exists") {
		return &native.Failure{
			Message:     "AWS already has a Redshift Serverless endpoint with the EndpointName, which an earlier attempt of this deploy may have created. Deploy again to manage it, or choose another name: " + err.Error(),
			ClientError: true,
			Outcome:     native.OutcomeUnknown,
		}
	}
	return rejected("The workgroup "+workgroup, err)
}

// Update changes the endpoint's security groups. AWS rejects a call that sends the groups the endpoint already has,
// so it sends nothing when the template's groups are AWS's.
func (h *EndpointAccess) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := validateEndpoint(request.Desired); err != nil {
		return err
	}
	if !slices.Contains(awsapi.Changed(request), "VpcSecurityGroupIds") {
		return nil
	}
	groups, _ := stringList(endpointAccessType, request.Desired, "VpcSecurityGroupIds")
	if len(groups) == 0 {
		return invalid(endpointAccessType, "VpcSecurityGroupIds", "at least one security group, because AWS can't remove the last one")
	}
	e, err := h.get(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	if sameSet(groups, securityGroups(e)) {
		return nil
	}
	_, err = h.api.UpdateEndpointAccess(ctx, &redshiftserverless.UpdateEndpointAccessInput{
		EndpointName:        aws.String(request.Identifier),
		VpcSecurityGroupIds: groups,
	}, inRegion(request.Region))
	if awsapi.HasCode(err, conflict) {
		// AWS changes only an ACTIVE endpoint. An SDK retry after a lost response finds the endpoint MODIFYING to
		// the groups the first attempt sent, so the update reached AWS.
		e, getErr := h.get(ctx, request.Region, request.Identifier)
		if getErr != nil {
			return unconfirmed(err, getErr)
		}
		if sameSet(groups, securityGroups(e)) {
			return nil
		}
	}
	return rejected("The endpoint "+request.Identifier, awsapi.NotFound(err, notFound))
}

// Delete deletes the endpoint. An endpoint AWS is already deleting had a delete reach AWS before, so the delete
// succeeds.
func (h *EndpointAccess) Delete(ctx context.Context, region string, identifier string) error {
	e, err := h.get(ctx, region, identifier)
	if err != nil {
		return err
	}
	if strings.EqualFold(aws.ToString(e.EndpointStatus), endpointDeleting) {
		return nil
	}
	_, err = h.api.DeleteEndpointAccess(ctx, &redshiftserverless.DeleteEndpointAccessInput{EndpointName: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, conflict) {
		// AWS deletes only an active endpoint, so another delete may have started after the read above.
		e, getErr := h.get(ctx, region, identifier)
		if getErr != nil {
			return getErr
		}
		if strings.EqualFold(aws.ToString(e.EndpointStatus), endpointDeleting) {
			return nil
		}
	}
	return rejected("The endpoint "+identifier, awsapi.NotFound(err, notFound))
}

// List returns every endpoint in the region.
func (h *EndpointAccess) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := redshiftserverless.NewListEndpointAccessPaginator(h.api, &redshiftserverless.ListEndpointAccessInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.Endpoints {
			e := &page.Endpoints[i]
			out = append(out, native.Resource{Identifier: aws.ToString(e.EndpointName), Properties: endpointProperties(e)})
		}
	}
	return out, nil
}

// Progress reads the endpoint's status. A create or an update waits for ACTIVE, and a delete for the endpoint to be
// gone.
func (h *EndpointAccess) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	e, err := h.get(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(e.EndpointStatus)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	return endpointStates.Progress(status), nil
}
