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

package quicksight

import (
	"context"
	"math"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/aws/aws-sdk-go-v2/service/quicksight/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	templateAliasType = "AWS::QuickSight::TemplateAlias"

	// maxTemplateIDLength and maxAliasLength are the documented maximum lengths of TemplateId and AliasName.
	maxTemplateIDLength = 512
	maxAliasLength      = 2048
)

var (
	_ native.Handler           = (*TemplateAlias)(nil)
	_ native.IdentifierEncoder = (*TemplateAlias)(nil)
)

// TemplateAlias serves AWS::QuickSight::TemplateAlias: a name that points to a version of a template. Its identifier
// is AwsAccountId, TemplateId, and AliasName, joined with "|". An update points the alias to another version, and a
// delete leaves the alias in AWS (see Delete). The identifier is known before the create, so a lost create is found by
// it. Names that start with $, such as $LATEST, are AWS's own and can't be managed.
type TemplateAlias struct {
	api API
}

// NewTemplateAlias returns the handler for Quick Sight template aliases.
func NewTemplateAlias(api API) *TemplateAlias {
	return &TemplateAlias{api: api}
}

// TypeName returns AWS::QuickSight::TemplateAlias.
func (h *TemplateAlias) TypeName() string {
	return templateAliasType
}

// Options returns how UCP uses the handler. AWS lists aliases only by template, so the type can't be listed.
func (h *TemplateAlias) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier, CreateTimeout: timeout, UpdateTimeout: timeout}
}

// aliasID names an alias: a name on a template of an account.
type aliasID struct {
	account, template, alias string
}

func (id aliasID) String() string {
	return strings.Join([]string{id.account, id.template, id.alias}, "|")
}

// parseAlias returns the alias the properties name. ok is false when they don't name all of it.
func parseAlias(properties map[string]any) (id aliasID, ok bool, err error) {
	if id.account, err = accountID(templateAliasType, properties); err != nil {
		return aliasID{}, false, err
	}
	if id.template, err = resourceID(templateAliasType, properties, "TemplateId", maxTemplateIDLength); err != nil {
		return aliasID{}, false, err
	}
	if id.alias, err = resourceID(templateAliasType, properties, "AliasName", maxAliasLength); err != nil {
		return aliasID{}, false, err
	}
	return id, id.account != "" && id.template != "" && id.alias != "", nil
}

// parseAliasID returns the alias an identifier names.
func parseAliasID(identifier string) (aliasID, error) {
	parts := strings.Split(identifier, "|")
	if len(parts) == 3 {
		id := aliasID{account: parts[0], template: parts[1], alias: parts[2]}
		if accountPattern.MatchString(id.account) && validID(id.template, maxTemplateIDLength) && validID(id.alias, maxAliasLength) {
			return id, nil
		}
	}
	return aliasID{}, invalid(templateAliasType, "%q is not the identifier of a Quick Sight template alias: it must be AwsAccountId|TemplateId|AliasName, with an AliasName that doesn't start with $", identifier)
}

// templateVersion returns the template's TemplateVersionNumber, which must be a whole number of at least 1.
func templateVersion(properties map[string]any) (int64, error) {
	var v float64
	switch n := properties["TemplateVersionNumber"].(type) {
	case float64:
		v = n
	case int:
		v = float64(n)
	case int64:
		v = float64(n)
	default:
		return 0, invalid(templateAliasType, "set TemplateVersionNumber to the number of a version of the template")
	}
	if v < 1 || v != math.Trunc(v) || v > math.MaxInt64 {
		return 0, invalid(templateAliasType, "TemplateVersionNumber must be a whole number of at least 1")
	}
	return int64(v), nil
}

// Identifier returns AwsAccountId|TemplateId|AliasName, or "" when the properties don't give all of them. UCP calls it
// before any AWS call, so a template with a value AWS would reject is rejected here.
func (h *TemplateAlias) Identifier(properties map[string]any) (string, error) {
	id, ok, err := parseAlias(properties)
	if err != nil || !ok {
		return "", err
	}
	if _, present := properties["TemplateVersionNumber"]; present {
		if _, err := templateVersion(properties); err != nil {
			return "", err
		}
	}
	return id.String(), nil
}

func (h *TemplateAlias) describe(ctx context.Context, region string, id aliasID) (*types.TemplateAlias, error) {
	out, err := h.api.DescribeTemplateAlias(ctx, &quicksight.DescribeTemplateAliasInput{
		AwsAccountId: aws.String(id.account),
		TemplateId:   aws.String(id.template),
		AliasName:    aws.String(id.alias),
	}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "ResourceNotFoundException")
	}
	if out.TemplateAlias == nil {
		return nil, native.ErrNotFound
	}
	return out.TemplateAlias, nil
}

// Read returns the alias's properties.
func (h *TemplateAlias) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseAliasID(identifier)
	if err != nil {
		return nil, err
	}
	alias, err := h.describe(ctx, region, id)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{
		"AwsAccountId": id.account,
		"TemplateId":   id.template,
		"AliasName":    id.alias,
		"Arn":          aws.ToString(alias.Arn),
	}
	if alias.TemplateVersionNumber != nil {
		properties["TemplateVersionNumber"] = float64(*alias.TemplateVersionNumber)
	}
	return properties, nil
}

// point points the alias to version, for an alias that exists.
func (h *TemplateAlias) point(ctx context.Context, region string, id aliasID, version int64) error {
	_, err := h.api.UpdateTemplateAlias(ctx, &quicksight.UpdateTemplateAliasInput{
		AwsAccountId:          aws.String(id.account),
		TemplateId:            aws.String(id.template),
		AliasName:             aws.String(id.alias),
		TemplateVersionNumber: aws.Int64(version),
	}, inRegion(region))
	return startError(templateAliasType, "template", err)
}

// Create creates the alias. When AWS already has it, such as after a create whose response was lost, Create points it
// to the template's version instead, so it ends as the template sets it either way.
func (h *TemplateAlias) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, ok, err := parseAlias(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", invalid(templateAliasType, "set AwsAccountId, TemplateId, and AliasName")
	}
	version, err := templateVersion(request.Properties)
	if err != nil {
		return "", err
	}
	_, err = h.api.CreateTemplateAlias(ctx, &quicksight.CreateTemplateAliasInput{
		AwsAccountId:          aws.String(id.account),
		TemplateId:            aws.String(id.template),
		AliasName:             aws.String(id.alias),
		TemplateVersionNumber: aws.Int64(version),
	}, inRegion(request.Region))
	if awsapi.HasCode(err, "ResourceExistsException") {
		return id.String(), h.point(ctx, request.Region, id, version)
	}
	if err != nil {
		return "", startError(templateAliasType, "template", err)
	}
	return id.String(), nil
}

// Update points the alias to the template's version. AWS answers ResourceNotFoundException for a version the template
// doesn't have as well as for a missing alias, so it is returned as AWS's rejection, not as a missing alias.
func (h *TemplateAlias) Update(ctx context.Context, request native.UpdateRequest) error {
	id, err := parseAliasID(request.Identifier)
	if err != nil {
		return err
	}
	version, err := templateVersion(request.Desired)
	if err != nil {
		return err
	}
	return h.point(ctx, request.Region, id, version)
}

// Delete changes nothing. AWS's docs say DeleteTemplateAlias also deletes the template version the alias points to,
// which the template owns, so the type's metadata makes a delete leave the alias in AWS, and UCP never calls it.
func (h *TemplateAlias) Delete(ctx context.Context, region string, identifier string) error {
	return nil
}

// List returns nothing, because AWS lists aliases only by template.
func (h *TemplateAlias) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the alias, which AWS returns once the create or update call returns, so the operation is done once
// it reads. It returns ErrNotFound when AWS has no such alias.
func (h *TemplateAlias) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	id, err := parseAliasID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.describe(ctx, region, id); err != nil {
		return native.Progress{}, err
	}
	return native.Progress{Done: true, State: "found"}, nil
}
