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

// Package transfer has the native handler for AWS Transfer Family accesses, which have no CloudFormation type.
package transfer

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/transfer"
	"github.com/aws/aws-sdk-go-v2/service/transfer/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	accessType = "AWS::Transfer::Access"

	notFound = "ResourceNotFoundException"
)

// API is the part of the AWS Transfer Family client the handler uses.
type API interface {
	CreateAccess(ctx context.Context, params *transfer.CreateAccessInput, optFns ...func(*transfer.Options)) (*transfer.CreateAccessOutput, error)
	DescribeAccess(ctx context.Context, params *transfer.DescribeAccessInput, optFns ...func(*transfer.Options)) (*transfer.DescribeAccessOutput, error)
	UpdateAccess(ctx context.Context, params *transfer.UpdateAccessInput, optFns ...func(*transfer.Options)) (*transfer.UpdateAccessOutput, error)
	DeleteAccess(ctx context.Context, params *transfer.DeleteAccessInput, optFns ...func(*transfer.Options)) (*transfer.DeleteAccessOutput, error)
}

var inRegion = awsapi.InRegion[transfer.Options]

var (
	_ native.Handler           = (*Access)(nil)
	_ native.IdentifierEncoder = (*Access)(nil)
)

// Access serves AWS::Transfer::Access: the access a directory group has to a Transfer Family server. Its identifier
// is ServerId and ExternalId joined with "|". AWS changes an access in place, so only those two are create-only.
type Access struct {
	api API
}

// NewAccess returns the handler for Transfer Family accesses.
func NewAccess(api API) *Access {
	return &Access{api: api}
}

// TypeName returns AWS::Transfer::Access.
func (h *Access) TypeName() string {
	return accessType
}

// Options returns how UCP uses the handler. AWS lists accesses only by server, so the type can't be listed.
func (h *Access) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// accessID names an access.
type accessID struct {
	server, external string
}

func (id accessID) String() string {
	return id.server + "|" + id.external
}

func (id accessID) describeInput() *transfer.DescribeAccessInput {
	return &transfer.DescribeAccessInput{ServerId: aws.String(id.server), ExternalId: aws.String(id.external)}
}

// clientFailure is a failure the template caused, which AWS never saw.
func clientFailure(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

func wrongType(path string, want string) error {
	return clientFailure("%s of an AWS::Transfer::Access must be %s", path, want)
}

// values are the template's values, as the API takes them. A value the template leaves out is nil.
type values struct {
	id                                             accessID
	role, homeDirectory, homeDirectoryType, policy *string
	mappings                                       []types.HomeDirectoryMapEntry
	posix                                          *types.PosixProfile
}

// optionalString returns the string property name of properties, or nil when it is absent.
func optionalString(properties map[string]any, name string, path string) (*string, error) {
	value, ok := properties[name]
	if !ok || value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok {
		return nil, wrongType(path, "a string")
	}
	return &s, nil
}

// posixID returns the integer v holds, when it is one of 0 to 4294967295, the range of a POSIX ID.
func posixID(v any) (int64, bool) {
	n, ok := v.(float64)
	if !ok || n != math.Trunc(n) || n < 0 || n > math.MaxUint32 {
		return 0, false
	}
	return int64(n), true
}

// mappingsOf returns the HomeDirectoryMappings of properties, or nil when it is absent.
func mappingsOf(properties map[string]any) ([]types.HomeDirectoryMapEntry, error) {
	value, ok := properties["HomeDirectoryMappings"]
	if !ok || value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, wrongType("HomeDirectoryMappings", "a list of mappings")
	}
	out := []types.HomeDirectoryMapEntry{}
	for _, item := range list {
		mapping, ok := item.(map[string]any)
		if !ok {
			return nil, wrongType("Each of HomeDirectoryMappings", "an object with Entry and Target")
		}
		entry, okEntry := mapping["Entry"].(string)
		target, okTarget := mapping["Target"].(string)
		if !okEntry || !okTarget {
			return nil, wrongType("The Entry and Target of each of HomeDirectoryMappings", "strings")
		}
		typ, err := optionalString(mapping, "Type", "The Type of each of HomeDirectoryMappings")
		if err != nil {
			return nil, err
		}
		out = append(out, types.HomeDirectoryMapEntry{Entry: aws.String(entry), Target: aws.String(target), Type: types.MapType(aws.ToString(typ))})
	}
	return out, nil
}

// posixOf returns the PosixProfile of properties, or nil when it is absent. Its SecondaryGids are never nil, so an
// update that sends the profile clears any secondary groups the template no longer sets.
func posixOf(properties map[string]any) (*types.PosixProfile, error) {
	value, ok := properties["PosixProfile"]
	if !ok || value == nil {
		return nil, nil
	}
	profile, ok := value.(map[string]any)
	if !ok {
		return nil, wrongType("PosixProfile", "an object with Uid and Gid")
	}
	uid, okUID := posixID(profile["Uid"])
	gid, okGID := posixID(profile["Gid"])
	if !okUID || !okGID {
		return nil, wrongType("The Uid and Gid of PosixProfile", "integers from 0 to 4294967295")
	}
	out := &types.PosixProfile{Uid: aws.Int64(uid), Gid: aws.Int64(gid), SecondaryGids: []int64{}}
	if value, ok := profile["SecondaryGids"]; ok && value != nil {
		list, ok := value.([]any)
		if !ok {
			return nil, wrongType("SecondaryGids of PosixProfile", "a list of integers from 0 to 4294967295")
		}
		for _, v := range list {
			n, ok := posixID(v)
			if !ok {
				return nil, wrongType("SecondaryGids of PosixProfile", "a list of integers from 0 to 4294967295")
			}
			out.SecondaryGids = append(out.SecondaryGids, n)
		}
	}
	return out, nil
}

// parse returns the template's values. A value of the wrong type, or a ServerId or ExternalId with "|", which
// separates them in the identifier, is a client failure.
func parse(properties map[string]any) (values, error) {
	v := values{}
	for _, field := range []struct {
		name  string
		value **string
	}{{"Role", &v.role}, {"HomeDirectory", &v.homeDirectory}, {"HomeDirectoryType", &v.homeDirectoryType}, {"Policy", &v.policy}} {
		s, err := optionalString(properties, field.name, field.name)
		if err != nil {
			return values{}, err
		}
		*field.value = s
	}
	for _, field := range []struct {
		name  string
		value *string
	}{{"ServerId", &v.id.server}, {"ExternalId", &v.id.external}} {
		s, err := optionalString(properties, field.name, field.name)
		if err != nil {
			return values{}, err
		}
		value := aws.ToString(s)
		if strings.Contains(value, "|") {
			return values{}, clientFailure("%s of an AWS::Transfer::Access can't contain \"|\", which Radius uses to join ServerId and ExternalId into the access's identifier", field.name)
		}
		*field.value = value
	}
	var err error
	if v.mappings, err = mappingsOf(properties); err != nil {
		return values{}, err
	}
	if v.posix, err = posixOf(properties); err != nil {
		return values{}, err
	}
	return v, nil
}

// parseID returns the access an identifier names.
func parseID(identifier string) (accessID, error) {
	server, external, ok := strings.Cut(identifier, "|")
	if !ok || server == "" || external == "" || strings.Contains(external, "|") {
		return accessID{}, clientFailure("%q is not the identifier of an AWS Transfer Family access: it must be ServerId|ExternalId", identifier)
	}
	return accessID{server: server, external: external}, nil
}

// Identifier returns ServerId|ExternalId, or "" when the properties don't give both. UCP calls it before any AWS call,
// so a template with a value of the wrong type is rejected here.
func (h *Access) Identifier(properties map[string]any) (string, error) {
	v, err := parse(properties)
	if err != nil || v.id.server == "" || v.id.external == "" {
		return "", err
	}
	return v.id.String(), nil
}

// describe returns the access an identifier names, or ErrNotFound when the server or the access doesn't exist.
func (h *Access) describe(ctx context.Context, region string, identifier string) (accessID, *types.DescribedAccess, error) {
	id, err := parseID(identifier)
	if err != nil {
		return accessID{}, nil, err
	}
	out, err := h.api.DescribeAccess(ctx, id.describeInput(), inRegion(region))
	if err != nil {
		return accessID{}, nil, awsapi.NotFound(err, notFound)
	}
	return id, out.Access, nil
}

// Read returns the access's properties. HomeDirectory and Policy are always set, and empty when the access has none,
// and so are a POSIX profile's SecondaryGids, so a template that sets one empty matches AWS instead of sending an update
// on every deploy.
func (h *Access) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, access, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{
		"ServerId":      id.server,
		"ExternalId":    id.external,
		"HomeDirectory": aws.ToString(access.HomeDirectory),
		"Policy":        aws.ToString(access.Policy),
	}
	for name, value := range map[string]string{"Role": aws.ToString(access.Role), "HomeDirectoryType": string(access.HomeDirectoryType)} {
		if value != "" {
			properties[name] = value
		}
	}
	if len(access.HomeDirectoryMappings) > 0 {
		mappings := []any{}
		for _, m := range access.HomeDirectoryMappings {
			mapping := map[string]any{"Entry": aws.ToString(m.Entry), "Target": aws.ToString(m.Target)}
			if m.Type != "" {
				mapping["Type"] = string(m.Type)
			}
			mappings = append(mappings, mapping)
		}
		properties["HomeDirectoryMappings"] = mappings
	}
	if p := access.PosixProfile; p != nil {
		gids := []any{}
		for _, gid := range p.SecondaryGids {
			gids = append(gids, float64(gid))
		}
		properties["PosixProfile"] = map[string]any{"Uid": float64(aws.ToInt64(p.Uid)), "Gid": float64(aws.ToInt64(p.Gid)), "SecondaryGids": gids}
	}
	return properties, nil
}

// Create creates the access.
func (h *Access) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	v, err := parse(request.Properties)
	if err != nil {
		return "", err
	}
	if v.id.server == "" || v.id.external == "" || v.role == nil {
		return "", clientFailure("an AWS::Transfer::Access needs ServerId, ExternalId, and Role")
	}
	_, err = h.api.CreateAccess(ctx, &transfer.CreateAccessInput{
		ServerId:              aws.String(v.id.server),
		ExternalId:            aws.String(v.id.external),
		Role:                  v.role,
		HomeDirectory:         v.homeDirectory,
		HomeDirectoryType:     types.HomeDirectoryType(aws.ToString(v.homeDirectoryType)),
		HomeDirectoryMappings: v.mappings,
		Policy:                v.policy,
		PosixProfile:          v.posix,
	}, inRegion(request.Region))
	if awsapi.HasCode(err, "ResourceExistsException") {
		// UCP reads AWS by the identifier before it creates an access, so this access may be the one an earlier
		// attempt of this create made, such as when the SDK retried a call whose response was lost. The outcome is
		// then unknown, so UCP keeps the record and the next deploy finds the access by its identifier.
		return "", &native.Failure{
			Message:     "the server already has an access for the template's ExternalId (ResourceExistsException), which an earlier attempt of this deploy may have created. Deploy again to manage it",
			ClientError: true,
			Outcome:     native.OutcomeUnknown,
		}
	}
	if err != nil {
		return "", err
	}
	return v.id.String(), nil
}

// cantRemove is the failure for a template that removes a property UpdateAccess has no way to clear.
func cantRemove(name string) error {
	return clientFailure("AWS can't remove %s from a Transfer Family access. Set it in the template, or delete the access and deploy it again", name)
}

// Update sends the properties the patch changes in one UpdateAccess call. A removed HomeDirectory or Policy is sent
// empty and a removed HomeDirectoryType as PATH, AWS's default, which clears them. AWS has no value that clears
// HomeDirectoryMappings or PosixProfile, so removing one fails before any change.
func (h *Access) Update(ctx context.Context, request native.UpdateRequest) error {
	v, err := parse(request.Desired)
	if err != nil {
		return err
	}
	id, err := parseID(request.Identifier)
	if err != nil {
		return err
	}
	input := &transfer.UpdateAccessInput{ServerId: aws.String(id.server), ExternalId: aws.String(id.external)}
	send := false
	for _, name := range awsapi.Changed(request) {
		switch name {
		case "Role":
			if v.role == nil {
				return cantRemove(name)
			}
			input.Role = v.role
		case "HomeDirectory":
			input.HomeDirectory = aws.String(aws.ToString(v.homeDirectory))
		case "HomeDirectoryType":
			input.HomeDirectoryType = types.HomeDirectoryTypePath
			if v.homeDirectoryType != nil {
				input.HomeDirectoryType = types.HomeDirectoryType(*v.homeDirectoryType)
			}
		case "Policy":
			input.Policy = aws.String(aws.ToString(v.policy))
		case "HomeDirectoryMappings":
			if v.mappings == nil {
				return cantRemove(name)
			}
			input.HomeDirectoryMappings = v.mappings
		case "PosixProfile":
			if v.posix == nil {
				return cantRemove(name)
			}
			input.PosixProfile = v.posix
		default:
			continue
		}
		send = true
	}
	if !send {
		return nil
	}
	_, err = h.api.UpdateAccess(ctx, input, inRegion(request.Region))
	return awsapi.NotFound(err, notFound)
}

// Delete deletes the access.
func (h *Access) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseID(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteAccess(ctx, &transfer.DeleteAccessInput{ServerId: aws.String(id.server), ExternalId: aws.String(id.external)}, inRegion(region))
	return awsapi.NotFound(err, notFound)
}

// List returns nothing, because AWS lists accesses only by server.
func (h *Access) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the access. AWS creates, changes, and deletes an access in the call, so a create or an update is
// done once AWS has the access, and a delete once it doesn't, which is ErrNotFound.
func (h *Access) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if _, _, err := h.describe(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "exists", Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true}, nil
}
