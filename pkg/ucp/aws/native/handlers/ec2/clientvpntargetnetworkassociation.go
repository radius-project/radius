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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const clientVpnTargetNetworkAssociationType = "AWS::EC2::ClientVpnTargetNetworkAssociation"

var _ native.Handler = (*ClientVpnTargetNetworkAssociation)(nil)

// associationNotFound are the codes AWS returns when the endpoint or the association doesn't exist.
var associationNotFound = []string{"InvalidClientVpnEndpointId.NotFound", "InvalidClientVpnAssociationIdNotFound"}

// associationStates are the states a create waits for.
var associationStates = awsapi.States{
	Done:   []string{string(types.AssociationStatusCodeAssociated)},
	Failed: []string{string(types.AssociationStatusCodeAssociationFailed)},
}

// ClientVpnTargetNetworkAssociation serves AWS::EC2::ClientVpnTargetNetworkAssociation, a subnet associated with a
// Client VPN endpoint. Its identifier is ClientVpnEndpointId and AssociationId joined with "|", such as
// cvpn-endpoint-0123456789abcdef0|cvpn-assoc-0123456789abcdef0. AWS assigns the association ID, and
// AssociateClientVpnTargetNetwork takes the record's client token as its idempotency token, so a create sent again
// returns the first association. Every property is create-only.
type ClientVpnTargetNetworkAssociation struct {
	api API
}

// NewClientVpnTargetNetworkAssociation returns the handler for Client VPN target network associations.
func NewClientVpnTargetNetworkAssociation(api API) *ClientVpnTargetNetworkAssociation {
	return &ClientVpnTargetNetworkAssociation{api: api}
}

// TypeName returns AWS::EC2::ClientVpnTargetNetworkAssociation.
func (h *ClientVpnTargetNetworkAssociation) TypeName() string {
	return clientVpnTargetNetworkAssociationType
}

// Options returns how UCP uses the handler. Associating and disassociating a subnet can take several minutes.
func (h *ClientVpnTargetNetworkAssociation) Options() native.Options {
	return native.Options{
		// AWS lists associations only per endpoint, which UCP's list can't name.
		List:          native.ListParent,
		Recovery:      native.RecoverByResend,
		CreateTimeout: 130 * time.Minute,
		DeleteTimeout: 130 * time.Minute,
	}
}

func parseAssociation(identifier string) (endpoint string, association string, err error) {
	endpoint, association, ok := strings.Cut(identifier, "|")
	if !ok || endpoint == "" || association == "" || strings.Contains(association, "|") {
		return "", "", invalidIdentifier(clientVpnTargetNetworkAssociationType, identifier, "ClientVpnEndpointId|AssociationId")
	}
	return endpoint, association, nil
}

// describe returns the association with the identifier, or ErrNotFound when AWS has none or it is disassociated.
func (h *ClientVpnTargetNetworkAssociation) describe(ctx context.Context, region string, identifier string) (*types.TargetNetwork, error) {
	endpoint, association, err := parseAssociation(identifier)
	if err != nil {
		return nil, err
	}
	out, err := h.api.DescribeClientVpnTargetNetworks(ctx, &awsec2.DescribeClientVpnTargetNetworksInput{
		ClientVpnEndpointId: aws.String(endpoint),
		AssociationIds:      []string{association},
	}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, associationNotFound...)
	}
	for _, n := range out.ClientVpnTargetNetworks {
		if aws.ToString(n.AssociationId) != association {
			continue
		}
		if n.Status != nil && n.Status.Code == types.AssociationStatusCodeDisassociated {
			return nil, native.ErrNotFound
		}
		return &n, nil
	}
	return nil, native.ErrNotFound
}

// Read returns the association's properties. AWS doesn't return the Availability Zone the template may have set
// instead of a subnet, so AvailabilityZone and AvailabilityZoneId are write-only.
func (h *ClientVpnTargetNetworkAssociation) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	n, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	p := map[string]any{
		"AssociationId":       n.AssociationId,
		"ClientVpnEndpointId": n.ClientVpnEndpointId,
	}
	if target := aws.ToString(n.TargetNetworkId); strings.HasPrefix(target, "subnet-") {
		p["SubnetId"] = target
	}
	return awsapi.EncodeObject(p)
}

// Create associates the subnet or Availability Zone with the endpoint, and returns the identifier.
func (h *ClientVpnTargetNetworkAssociation) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	input := &awsec2.AssociateClientVpnTargetNetworkInput{}
	if err := awsapi.Decode(request.Properties, input); err != nil {
		return "", err
	}
	input.ClientToken = aws.String(request.ClientToken)
	out, err := h.api.AssociateClientVpnTargetNetwork(ctx, input, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	if aws.ToString(out.AssociationId) == "" {
		return "", fmt.Errorf("AWS returned no association ID for the create of an AWS resource of type %s", clientVpnTargetNetworkAssociationType)
	}
	return aws.ToString(input.ClientVpnEndpointId) + "|" + aws.ToString(out.AssociationId), nil
}

// Update does nothing: every property of an association is create-only.
func (h *ClientVpnTargetNetworkAssociation) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete disassociates the target network. An association AWS is already disassociating had a delete reach AWS
// before, so the delete succeeds.
func (h *ClientVpnTargetNetworkAssociation) Delete(ctx context.Context, region string, identifier string) error {
	n, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	if n.Status != nil && n.Status.Code == types.AssociationStatusCodeDisassociating {
		return nil
	}
	endpoint, association, _ := parseAssociation(identifier)
	_, err = h.api.DisassociateClientVpnTargetNetwork(ctx, &awsec2.DisassociateClientVpnTargetNetworkInput{
		ClientVpnEndpointId: aws.String(endpoint),
		AssociationId:       aws.String(association),
	}, inRegion(region))
	return awsapi.NotFound(err, associationNotFound...)
}

// List is not supported: AWS lists associations only per endpoint (native.ListParent). UCP calls it only for types it
// can list in a region.
func (h *ClientVpnTargetNetworkAssociation) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", clientVpnTargetNetworkAssociationType)
}

// Progress reads the association's status. A create waits for associated, and a delete for disassociated.
func (h *ClientVpnTargetNetworkAssociation) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	n, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := ""
	if n.Status != nil {
		status = string(n.Status.Code)
	}
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: string(types.AssociationStatusCodeDisassociated)}, nil
	}
	return associationStates.Progress(status), nil
}
