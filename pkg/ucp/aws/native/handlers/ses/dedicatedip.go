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

package ses

import (
	"context"
	"errors"
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	dedicatedIPType = "AWS::SES::DedicatedIpAssignment"

	// poolProperty is the property that names the pool the address sends from.
	poolProperty = "DestinationPoolName"
)

var (
	_ native.Handler           = (*DedicatedIPAssignment)(nil)
	_ native.IdentifierEncoder = (*DedicatedIPAssignment)(nil)
)

// DedicatedIPAssignment serves AWS::SES::DedicatedIpAssignment, the pool an SES dedicated IP address sends from. Its
// identifier is the address. Every address the account leases is in a pool, so UCP takes the assignment over and
// updates it, and calls Create only for an address SES doesn't have, which fails. SES has no call that takes an
// address out of a pool, so the type's metadata makes a delete call nothing in AWS.
type DedicatedIPAssignment struct {
	api  V2API
	wait pause
}

// NewDedicatedIPAssignment returns the handler for the pool of a dedicated IP address.
func NewDedicatedIPAssignment(api V2API) *DedicatedIPAssignment {
	return &DedicatedIPAssignment{api: api, wait: sleep}
}

// TypeName returns AWS::SES::DedicatedIpAssignment.
func (h *DedicatedIPAssignment) TypeName() string {
	return dedicatedIPType
}

// Options returns how UCP uses the handler.
func (h *DedicatedIPAssignment) Options() native.Options {
	return options()
}

// Identifier returns Ip. An address that isn't an IPv4 address in its usual form, such as one with leading zeros, is
// rejected before any call, so one address has one identifier.
func (h *DedicatedIPAssignment) Identifier(properties map[string]any) (string, error) {
	ip := awsapi.String(properties, "Ip")
	if parsed := net.ParseIP(ip); ip != "" && (parsed.To4() == nil || parsed.String() != ip) {
		return "", rejected("Ip must be an IPv4 address, such as 198.51.100.7")
	}
	return ip, nil
}

// Read returns the address and its pool, or ErrNotFound when SES has no such address in the account.
func (h *DedicatedIPAssignment) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.api.GetDedicatedIp(ctx, &sesv2.GetDedicatedIpInput{Ip: aws.String(identifier)}, inRegionV2(region))
	if err != nil {
		return nil, readError(err)
	}
	properties := map[string]any{"Ip": identifier}
	if out.DedicatedIp != nil && aws.ToString(out.DedicatedIp.PoolName) != "" {
		properties[poolProperty] = aws.ToString(out.DedicatedIp.PoolName)
	}
	return properties, nil
}

// put moves the address to the pool properties name, and waits until a read shows it there. A move no read shows
// fails with an unknown outcome.
func (h *DedicatedIPAssignment) put(ctx context.Context, region string, ip string, properties map[string]any) error {
	pool := awsapi.String(properties, poolProperty)
	if pool == "" {
		return rejected("set %s", poolProperty)
	}
	_, err := h.api.PutDedicatedIpInPool(ctx, &sesv2.PutDedicatedIpInPoolInput{Ip: aws.String(ip), DestinationPoolName: aws.String(pool)}, inRegionV2(region))
	if err != nil {
		return awsapi.Rejected(err)
	}
	return confirm(ctx, h.wait, h.Read, region, ip, "the pool of the dedicated IP address", func(got map[string]any) bool {
		return got[poolProperty] == pool
	})
}

// Create moves the address to the template's pool and returns the address. UCP calls it only when a read found no
// such address, so SES rejects it.
func (h *DedicatedIPAssignment) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	ip := awsapi.String(request.Properties, "Ip")
	if err := h.put(ctx, request.Region, ip, request.Properties); err != nil {
		var failure *native.Failure
		if errors.As(err, &failure) && failure.Outcome == native.OutcomeUnknown {
			return ip, err
		}
		return "", err
	}
	return ip, nil
}

// Update moves the address to the template's pool.
func (h *DedicatedIPAssignment) Update(ctx context.Context, request native.UpdateRequest) error {
	return h.put(ctx, request.Region, request.Identifier, request.Desired)
}

// Delete changes nothing, because SES has no call that takes an address out of a pool. The type's metadata makes UCP
// remove only the record.
func (h *DedicatedIPAssignment) Delete(ctx context.Context, region string, identifier string) error {
	return nil
}

// List fails, because the type can't be listed.
func (h *DedicatedIPAssignment) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(dedicatedIPType)
}

// Progress reports every operation as done, because a create or an update waited until a read showed it, and a
// delete changes nothing.
func (h *DedicatedIPAssignment) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
