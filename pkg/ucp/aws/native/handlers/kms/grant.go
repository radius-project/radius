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

// Package kms has the native handler for AWS KMS grants, which have no CloudFormation type.
package kms

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const grantType = "AWS::KMS::Grant"

// API is the part of the AWS KMS client the handler uses.
type API interface {
	CreateGrant(ctx context.Context, params *kms.CreateGrantInput, optFns ...func(*kms.Options)) (*kms.CreateGrantOutput, error)
	ListGrants(ctx context.Context, params *kms.ListGrantsInput, optFns ...func(*kms.Options)) (*kms.ListGrantsOutput, error)
	RetireGrant(ctx context.Context, params *kms.RetireGrantInput, optFns ...func(*kms.Options)) (*kms.RetireGrantOutput, error)
	DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
}

var inRegion = awsapi.InRegion[kms.Options]

var (
	_ native.Handler            = (*Grant)(nil)
	_ native.IdentifierEncoder  = (*Grant)(nil)
	_ native.IdentifierResolver = (*Grant)(nil)
)

// grantNotFound are the codes AWS returns when the key or the grant doesn't exist. The docs don't say which one a call
// that names a grant AWS no longer has gets, so both mean the grant is gone.
var grantNotFound = []string{"NotFoundException", "InvalidGrantIdException"}

// Grant serves AWS::KMS::Grant: a grant on a KMS key. Its identifier is the key ARN and the grant ID AWS assigns,
// joined with "|". AWS has no call that changes a grant, so every property is create-only, and a delete retires it.
//
// CreateGrant takes no idempotency token, but a create with the same Name and values as an existing grant returns
// that grant. The handler names every grant from the record's client token, so the name belongs to one create, and a
// create sent again returns that create's grant, even before ListGrants lists it.
type Grant struct {
	api API
}

// NewGrant returns the handler for KMS grants.
func NewGrant(api API) *Grant {
	return &Grant{api: api}
}

// TypeName returns AWS::KMS::Grant.
func (h *Grant) TypeName() string {
	return grantType
}

// Options returns how UCP uses the handler. AWS lists grants only by key, which UCP's list can't name. A grant AWS
// just created or retired can take a few minutes to show the change in ListGrants, so a create and a delete wait for
// it.
func (h *Grant) Options() native.Options {
	return native.Options{
		List:          native.ListParent,
		Recovery:      native.RecoverByResend,
		Equivalent:    sameKey,
		CreateTimeout: 15 * time.Minute,
		DeleteTimeout: 15 * time.Minute,
	}
}

// grant is a grant's properties, in the schema's format.
type grant struct {
	KeyId                    string                  `json:",omitempty"`
	GrantId                  string                  `json:",omitempty"`
	Name                     string                  `json:",omitempty"`
	GranteePrincipal         string                  `json:",omitempty"`
	GranteeServicePrincipal  string                  `json:",omitempty"`
	RetiringPrincipal        string                  `json:",omitempty"`
	RetiringServicePrincipal string                  `json:",omitempty"`
	Operations               []types.GrantOperation  `json:",omitempty"`
	Constraints              *types.GrantConstraints `json:",omitempty"`
	IssuingAccount           string                  `json:",omitempty"`
}

// rejected is a client failure AWS never saw.
func rejected(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// isAlias reports whether key names a KMS alias, such as alias/app or its ARN, which CreateGrant doesn't take.
func isAlias(key string) bool {
	return strings.HasPrefix(key, "alias/") || strings.Contains(key, ":alias/")
}

// parse returns the grant the properties give. A value of the wrong type, a key alias, or an operation listed twice
// is a client failure. UCP calls Identifier before any AWS call, so such a template is rejected before AWS sees it.
func parse(properties map[string]any) (grant, error) {
	var g grant
	if err := awsapi.Decode(properties, &g); err != nil {
		return grant{}, rejected("the properties of an %s don't have the types its schema gives: %v", grantType, err)
	}
	if isAlias(g.KeyId) {
		return grant{}, rejected("KeyId of an %s must be a key ID or key ARN, because CreateGrant doesn't take an alias", grantType)
	}
	for i, op := range g.Operations {
		if slices.Contains(g.Operations[:i], op) {
			return grant{}, rejected("Operations of an %s lists %s more than once", grantType, op)
		}
	}
	return g, nil
}

// parseID returns the key and the grant ID of an identifier.
func parseID(identifier string) (string, string, error) {
	key, id, ok := strings.Cut(identifier, "|")
	if !ok || key == "" || id == "" || strings.Contains(id, "|") || isAlias(key) {
		return "", "", rejected("%q is not the identifier of an AWS KMS grant: it must be the key ID or key ARN and the grant ID, joined with \"|\"", identifier)
	}
	return key, id, nil
}

// Identifier returns KeyId|GrantId, or "" before AWS assigns the grant's ID. Properties without GrantId are a template
// to create, which needs KeyId and Operations.
func (h *Grant) Identifier(properties map[string]any) (string, error) {
	g, err := parse(properties)
	if err != nil {
		return "", err
	}
	if g.GrantId == "" {
		return "", required(g)
	}
	return g.KeyId + "|" + g.GrantId, nil
}

// required returns a client failure when the template leaves out KeyId or Operations, which the AWS SDK would reject
// before it sends the create, or sets Name, which the handler sets.
func required(g grant) error {
	if g.KeyId == "" || len(g.Operations) == 0 {
		return rejected("an %s needs KeyId and Operations", grantType)
	}
	if g.Name != "" {
		return rejected("Name of an %s is read-only: Radius names the grant, so a create it sends again returns the same grant", grantType)
	}
	return nil
}

// ResolveIdentifier returns the identifier with the key's ARN, the form AWS reports a grant's key in, so a name that
// gives the key ID names the same grant.
func (h *Grant) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	key, id, err := parseID(identifier)
	if err != nil {
		return "", err
	}
	arn, err := h.keyARN(ctx, region, key)
	if err != nil {
		return "", err
	}
	return arn + "|" + id, nil
}

// sameKey treats the key ARN AWS reports as the key ID the template sets.
func sameKey(path string, template any, reported any) bool {
	t, okT := template.(string)
	a, okA := reported.(string)
	return path == "KeyId" && okT && okA && strings.HasSuffix(a, ":key/"+t)
}

// keyARN returns the ARN of the key, which is key itself when it is an ARN.
func (h *Grant) keyARN(ctx context.Context, region string, key string) (string, error) {
	if strings.HasPrefix(key, "arn:") {
		return key, nil
	}
	out, err := h.api.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(key)}, inRegion(region))
	if err != nil {
		return "", err
	}
	if out.KeyMetadata == nil || aws.ToString(out.KeyMetadata.Arn) == "" {
		return "", fmt.Errorf("AWS described the KMS key %q without its ARN", key)
	}
	return aws.ToString(out.KeyMetadata.Arn), nil
}

// grants returns the grants ListGrants returns for in, from every page.
func (h *Grant) grants(ctx context.Context, region string, in *kms.ListGrantsInput) ([]types.GrantListEntry, error) {
	in.Limit = aws.Int32(100)
	out := []types.GrantListEntry{}
	for {
		page, err := h.api.ListGrants(ctx, in, inRegion(region))
		if err != nil {
			return nil, err
		}
		out = append(out, page.Grants...)
		if in.Marker = page.NextMarker; !page.Truncated || in.Marker == nil {
			return out, nil
		}
	}
}

// describe returns the grant with the identifier, or ErrNotFound when the key or the grant doesn't exist.
func (h *Grant) describe(ctx context.Context, region string, identifier string) (*types.GrantListEntry, error) {
	key, id, err := parseID(identifier)
	if err != nil {
		return nil, err
	}
	list, err := h.grants(ctx, region, &kms.ListGrantsInput{KeyId: aws.String(key), GrantId: aws.String(id)})
	if err != nil {
		return nil, awsapi.NotFound(err, grantNotFound...)
	}
	for i := range list {
		if aws.ToString(list[i].GrantId) == id {
			return &list[i], nil
		}
	}
	return nil, native.ErrNotFound
}

// Read returns the grant's properties.
func (h *Grant) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return awsapi.EncodeObject(grant{
		KeyId:                    aws.ToString(e.KeyId),
		GrantId:                  aws.ToString(e.GrantId),
		Name:                     aws.ToString(e.Name),
		GranteePrincipal:         aws.ToString(e.GranteePrincipal),
		GranteeServicePrincipal:  aws.ToString(e.GranteeServicePrincipal),
		RetiringPrincipal:        aws.ToString(e.RetiringPrincipal),
		RetiringServicePrincipal: aws.ToString(e.RetiringServicePrincipal),
		Operations:               e.Operations,
		Constraints:              e.Constraints,
		IssuingAccount:           aws.ToString(e.IssuingAccount),
	})
}

// named returns the identifiers of the key's grants with the name.
func (h *Grant) named(ctx context.Context, region string, key string, name string) ([]string, error) {
	list, err := h.grants(ctx, region, &kms.ListGrantsInput{KeyId: aws.String(key)})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, e := range list {
		if aws.ToString(e.Name) == name {
			ids = append(ids, aws.ToString(e.KeyId)+"|"+aws.ToString(e.GrantId))
		}
	}
	return ids, nil
}

// optional returns nil for "", so a value the template leaves out isn't sent.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return aws.String(s)
}

// Create creates the grant, named from the record's client token, and returns its identifier, with the key's ARN.
// The name belongs to this create, so a grant the key already has with the name is this create's: Create returns it,
// whatever its values, and UCP then reads it and compares it with the template. A create sent again before ListGrants
// lists the grant has the same name and, unless the template changed, the same values, so AWS returns the same grant.
//
// KMS rejects invalid input with ValidationError, which the AWS SDK doesn't know as a client error, so Create reports
// it as rejected, and UCP doesn't keep the record of a create AWS never ran.
func (h *Grant) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, err := h.create(ctx, request)
	if awsapi.HasCode(err, "ValidationError") {
		return "", &native.Failure{Message: err.Error(), ClientError: true, Outcome: native.OutcomeRejected}
	}
	return id, err
}

func (h *Grant) create(ctx context.Context, request native.CreateRequest) (string, error) {
	g, err := parse(request.Properties)
	if err == nil {
		err = required(g)
	}
	if err != nil {
		return "", err
	}
	name := native.GenerateName(request.ClientToken)
	key, err := h.keyARN(ctx, request.Region, g.KeyId)
	if err != nil {
		return "", err
	}
	existing, err := h.named(ctx, request.Region, key, name)
	switch {
	case err != nil:
		return "", err
	case len(existing) == 1:
		return existing[0], nil
	case len(existing) > 1:
		return "", fmt.Errorf("the KMS key has %d grants named %s, which only one create of an AWS resource of type %s makes, so UCP can't tell which one it made: %s", len(existing), name, grantType, strings.Join(existing, ", "))
	}
	out, err := h.api.CreateGrant(ctx, &kms.CreateGrantInput{
		KeyId:                    aws.String(key),
		Operations:               g.Operations,
		Constraints:              g.Constraints,
		Name:                     aws.String(name),
		GranteePrincipal:         optional(g.GranteePrincipal),
		GranteeServicePrincipal:  optional(g.GranteeServicePrincipal),
		RetiringPrincipal:        optional(g.RetiringPrincipal),
		RetiringServicePrincipal: optional(g.RetiringServicePrincipal),
	}, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	if aws.ToString(out.GrantId) == "" {
		return "", fmt.Errorf("AWS returned no grant ID for the create of an AWS resource of type %s", grantType)
	}
	return key + "|" + aws.ToString(out.GrantId), nil
}

// Update does nothing: every property of a grant is create-only, so UCP rejects a change before it calls Update.
func (h *Grant) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete retires the grant. The handler's credentials can retire a grant their account created.
func (h *Grant) Delete(ctx context.Context, region string, identifier string) error {
	e, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	_, err = h.api.RetireGrant(ctx, &kms.RetireGrantInput{KeyId: e.KeyId, GrantId: e.GrantId}, inRegion(region))
	return awsapi.NotFound(err, grantNotFound...)
}

// List is not supported: AWS lists grants only per key (native.ListParent). UCP calls it only for types it can list in
// a region.
func (h *Grant) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", grantType)
}

// Progress reads whether ListGrants shows the change yet. A create waits for the grant to be listed, and a delete for
// it to be gone. An update changes nothing, so it is done.
func (h *Grant) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	_, err := h.describe(ctx, region, identifier)
	switch {
	case op == native.OperationDelete && err == nil:
		return native.Progress{State: "listed", Waiting: "retired"}, nil
	case op == native.OperationCreate && errors.Is(err, native.ErrNotFound):
		return native.Progress{State: "not listed", Waiting: "listed"}, nil
	case err != nil:
		return native.Progress{}, err
	}
	return native.Progress{Done: true}, nil
}
