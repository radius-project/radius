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
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const vpcEndpointAccessType = "AWS::OpenSearchService::VpcEndpointAccess"

var (
	_ native.Handler           = (*VpcEndpointAccess)(nil)
	_ native.IdentifierEncoder = (*VpcEndpointAccess)(nil)
)

var (
	// domainNamePattern is the documented pattern of a domain name, which is 3 to 28 characters.
	domainNamePattern = regexp.MustCompile(`^[a-z][a-z0-9\-]+$`)

	// accountPattern is the documented pattern of Account.
	accountPattern = regexp.MustCompile(`^[0-9]+$`)

	// accountRoot matches an account's root principal ARN, the other form an IAM principal for an account takes.
	accountRoot = regexp.MustCompile(`^arn:aws[a-z\-]*:iam::([0-9]+):root$`)
)

// VpcEndpointAccess serves AWS::OpenSearchService::VpcEndpointAccess: an AWS account or service that a VPC domain lets
// create interface VPC endpoints to it. Its identifier is DomainName and the principal, the account ID or the service,
// joined with "|". AWS has no call that changes an authorization, so every property is create-only, and UCP rejects a
// change with 400 before any AWS call. The identifier is known before the create, so a lost create is found by it.
type VpcEndpointAccess struct {
	api API
}

// NewVpcEndpointAccess returns the handler for OpenSearch Service VPC endpoint access.
func NewVpcEndpointAccess(api API) *VpcEndpointAccess {
	return &VpcEndpointAccess{api: api}
}

// TypeName returns AWS::OpenSearchService::VpcEndpointAccess.
func (h *VpcEndpointAccess) TypeName() string {
	return vpcEndpointAccessType
}

// Options returns how UCP uses the handler. AWS lists access only by domain, so the type can't be listed.
func (h *VpcEndpointAccess) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// accessID names an authorization: a domain, and an account ID or a service principal.
type accessID struct {
	domain, principal string
	kind              types.PrincipalType
}

func (id accessID) String() string {
	return id.domain + "|" + id.principal
}

// principalOf returns the kind of a principal: one of the service principals AWS takes, or an account ID.
func principalOf(principal string) (types.PrincipalType, bool) {
	switch {
	case slices.Contains(types.AWSServicePrincipal("").Values(), types.AWSServicePrincipal(principal)):
		return types.PrincipalTypeAwsService, true
	case accountPattern.MatchString(principal):
		return types.PrincipalTypeAwsAccount, true
	}
	return "", false
}

// parseAccess returns the authorization the properties name. ok is false when they don't name all of it.
func parseAccess(properties map[string]any) (id accessID, ok bool, err error) {
	strs := map[string]string{}
	for _, name := range []string{"DomainName", "Account", "Service", "Principal"} {
		value, present := properties[name]
		if !present || value == nil {
			continue
		}
		s, isString := value.(string)
		if !isString {
			return accessID{}, false, invalid(vpcEndpointAccessType, "%s must be a string", name)
		}
		strs[name] = s
	}
	id.domain = strs["DomainName"]
	if id.domain != "" && (len(id.domain) < 3 || len(id.domain) > 28 || !domainNamePattern.MatchString(id.domain)) {
		return accessID{}, false, invalid(vpcEndpointAccessType, "DomainName must be 3 to 28 lowercase letters, digits, and hyphens, starting with a letter")
	}
	account, service := strs["Account"], strs["Service"]
	switch {
	case account != "" && service != "":
		return accessID{}, false, invalid(vpcEndpointAccessType, "set Account or Service, not both")
	case account != "":
		if !accountPattern.MatchString(account) {
			return accessID{}, false, invalid(vpcEndpointAccessType, "Account must be an AWS account ID")
		}
		id.principal, id.kind = account, types.PrincipalTypeAwsAccount
	case service != "":
		if kind, _ := principalOf(service); kind != types.PrincipalTypeAwsService {
			return accessID{}, false, invalid(vpcEndpointAccessType, "Service must be one of %v", types.AWSServicePrincipal("").Values())
		}
		id.principal, id.kind = service, types.PrincipalTypeAwsService
	}
	// Principal, the schema's primary identifier with DomainName, is read-only, but `existing` gives it. It names the
	// same principal as Account or Service.
	if principal := strs["Principal"]; principal != "" {
		kind, known := principalOf(principal)
		if !known || (id.principal != "" && id.principal != principal) {
			return accessID{}, false, invalid(vpcEndpointAccessType, "Principal must be the account ID or service principal that Account or Service gives")
		}
		id.principal, id.kind = principal, kind
	}
	if _, present := properties["ServiceOptions"]; present && id.kind == types.PrincipalTypeAwsAccount {
		return accessID{}, false, invalid(vpcEndpointAccessType, "ServiceOptions is for a Service, not an Account")
	}
	return id, id.domain != "" && id.principal != "", nil
}

// parseAccessID returns the authorization an identifier names.
func parseAccessID(identifier string) (accessID, error) {
	domain, principal, found := strings.Cut(identifier, "|")
	kind, ok := principalOf(principal)
	if !found || !ok || !domainNamePattern.MatchString(domain) {
		return accessID{}, invalid(vpcEndpointAccessType, "%q is not the identifier of an OpenSearch Service VPC endpoint access: it must be DomainName|Account or DomainName|Service", identifier)
	}
	return accessID{domain: domain, principal: principal, kind: kind}, nil
}

// Identifier returns DomainName|Account or DomainName|Service, or "" when the properties don't give both. Principal,
// which `existing` gives, stands for Account or Service. UCP calls it before any AWS call, so a template that sets both
// Account and Service, or a Principal that differs from them, is rejected here.
func (h *VpcEndpointAccess) Identifier(properties map[string]any) (string, error) {
	id, ok, err := parseAccess(properties)
	if err != nil || !ok {
		return "", err
	}
	return id.String(), nil
}

// sameKind reports whether AWS's PrincipalType names kind. AWS documents AWS_ACCOUNT and AWS_SERVICE, but other AWS
// clients report that it returns "AWS Account" for an account, so the comparison ignores case, spaces, and
// underscores.
func sameKind(reported types.PrincipalType, kind types.PrincipalType) bool {
	fold := strings.NewReplacer(" ", "", "_", "")
	return strings.EqualFold(fold.Replace(string(reported)), fold.Replace(string(kind)))
}

// matches reports whether AWS's principal p is the one id names. AWS's docs call Principal an IAM principal, so an
// account may come back as its ID or as its root ARN.
func (id accessID) matches(p types.AuthorizedPrincipal) bool {
	if !sameKind(p.PrincipalType, id.kind) {
		return false
	}
	principal := aws.ToString(p.Principal)
	if principal == id.principal {
		return true
	}
	m := accountRoot.FindStringSubmatch(principal)
	return id.kind == types.PrincipalTypeAwsAccount && m != nil && m[1] == id.principal
}

// find returns the domain's authorization of the principal, or ErrNotFound when the domain doesn't exist or doesn't
// list it.
func (h *VpcEndpointAccess) find(ctx context.Context, region string, id accessID) (*types.AuthorizedPrincipal, error) {
	input := &opensearch.ListVpcEndpointAccessInput{DomainName: aws.String(id.domain)}
	for {
		page, err := h.api.ListVpcEndpointAccess(ctx, input, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "ResourceNotFoundException")
		}
		for i := range page.AuthorizedPrincipalList {
			if id.matches(page.AuthorizedPrincipalList[i]) {
				return &page.AuthorizedPrincipalList[i], nil
			}
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return nil, native.ErrNotFound
		}
	}
}

// Read returns the authorization's properties.
func (h *VpcEndpointAccess) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseAccessID(identifier)
	if err != nil {
		return nil, err
	}
	p, err := h.find(ctx, region, id)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{
		"DomainName":    id.domain,
		"Principal":     id.principal,
		"PrincipalType": string(id.kind),
	}
	if id.kind == types.PrincipalTypeAwsAccount {
		properties["Account"] = id.principal
	} else {
		// A service always reads with ServiceOptions, with no Regions when AWS lists none, so a template that sets an
		// empty list redeploys with no change.
		regions := []string{}
		if p.ServiceOptions != nil {
			regions = p.ServiceOptions.SupportedRegions
		}
		properties["Service"] = id.principal
		properties["ServiceOptions"] = map[string]any{"SupportedRegions": stringsToAny(regions)}
	}
	return properties, nil
}

func stringsToAny(list []string) []any {
	out := make([]any, 0, len(list))
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

// serviceOptions returns the template's ServiceOptions, or nil when it has none.
func serviceOptions(properties map[string]any) (*types.ServiceOptions, error) {
	value, present := properties["ServiceOptions"]
	if !present || value == nil {
		return nil, nil
	}
	options, ok := value.(map[string]any)
	regions, listOK := stringList(options, "SupportedRegions")
	if !ok || !listOK {
		return nil, invalid(vpcEndpointAccessType, "ServiceOptions must be an object with SupportedRegions, a list of Regions")
	}
	return &types.ServiceOptions{SupportedRegions: regions}, nil
}

// sameServiceOptions returns a rejected client failure when the domain already authorizes the principal with other
// Regions than the template's, so a create never takes another authorization as the template's.
func sameServiceOptions(id accessID, want *types.ServiceOptions, got *types.ServiceOptions) error {
	regions := func(o *types.ServiceOptions) []string {
		if o == nil {
			return []string{}
		}
		return slices.Sorted(slices.Values(o.SupportedRegions))
	}
	if slices.Equal(regions(want), regions(got)) {
		return nil
	}
	return &native.Failure{
		Message: fmt.Sprintf("%s: the domain %q already authorizes %s with the SupportedRegions %v, not the template's %v. Set ServiceOptions to match, or revoke the authorization first",
			vpcEndpointAccessType, id.domain, id.principal, regions(got), regions(want)),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Create authorizes the principal. AWS lists an authorization as soon as it makes it, so Create reads the domain's
// list first and sends nothing when the principal is already there with the template's options, such as after a
// create whose response was lost.
func (h *VpcEndpointAccess) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, ok, err := parseAccess(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", invalid(vpcEndpointAccessType, "set DomainName, and exactly one of Account and Service")
	}
	options, err := serviceOptions(request.Properties)
	if err != nil {
		return "", err
	}
	switch p, err := h.find(ctx, request.Region, id); {
	case err == nil:
		if err := sameServiceOptions(id, options, p.ServiceOptions); err != nil {
			return "", err
		}
		return id.String(), nil
	case !errors.Is(err, native.ErrNotFound):
		return "", beforeChange(err)
	}
	input := &opensearch.AuthorizeVpcEndpointAccessInput{DomainName: aws.String(id.domain), ServiceOptions: options}
	if id.kind == types.PrincipalTypeAwsAccount {
		input.Account = aws.String(id.principal)
	} else {
		input.Service = types.AWSServicePrincipal(id.principal)
	}
	if _, err := h.api.AuthorizeVpcEndpointAccess(ctx, input, inRegion(request.Region), sendWithoutRetry); err != nil {
		return "", awsapi.Rejected(err)
	}
	return id.String(), nil
}

// Update changes nothing, because every property is create-only.
func (h *VpcEndpointAccess) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete revokes the principal's access, sending the service options AWS lists for it. It returns ErrNotFound when
// the domain doesn't list the principal.
func (h *VpcEndpointAccess) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseAccessID(identifier)
	if err != nil {
		return err
	}
	p, err := h.find(ctx, region, id)
	if err != nil {
		return beforeChange(err)
	}
	input := &opensearch.RevokeVpcEndpointAccessInput{DomainName: aws.String(id.domain), ServiceOptions: p.ServiceOptions}
	if id.kind == types.PrincipalTypeAwsAccount {
		input.Account = aws.String(id.principal)
	} else {
		input.Service = types.AWSServicePrincipal(id.principal)
	}
	_, err = h.api.RevokeVpcEndpointAccess(ctx, input, inRegion(region), sendWithoutRetry)
	return awsapi.Rejected(awsapi.NotFound(err, "ResourceNotFoundException"))
}

// List returns nothing, because AWS lists access only by domain.
func (h *VpcEndpointAccess) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the domain's list. A create is done once the principal is listed, and a delete once it isn't, when
// Progress returns ErrNotFound. An update changes nothing.
func (h *VpcEndpointAccess) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	id, err := parseAccessID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.find(ctx, region, id); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationCreate {
		return native.Progress{Done: true, State: "listed"}, nil
	}
	return native.Progress{State: "listed", Waiting: "not listed"}, nil
}
