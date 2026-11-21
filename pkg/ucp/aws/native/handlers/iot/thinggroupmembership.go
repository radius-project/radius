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

// Package iot has the native handler for AWS IoT thing group memberships, which have no CloudFormation type.
package iot

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iot"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const membershipType = "AWS::IoT::ThingGroupMembership"

// API is the part of the AWS IoT client the handler uses.
type API interface {
	AddThingToThingGroup(ctx context.Context, params *iot.AddThingToThingGroupInput, optFns ...func(*iot.Options)) (*iot.AddThingToThingGroupOutput, error)
	RemoveThingFromThingGroup(ctx context.Context, params *iot.RemoveThingFromThingGroupInput, optFns ...func(*iot.Options)) (*iot.RemoveThingFromThingGroupOutput, error)
	ListThingGroupsForThing(ctx context.Context, params *iot.ListThingGroupsForThingInput, optFns ...func(*iot.Options)) (*iot.ListThingGroupsForThingOutput, error)
}

var inRegion = awsapi.InRegion[iot.Options]

var (
	_ native.Handler           = (*ThingGroupMembership)(nil)
	_ native.IdentifierEncoder = (*ThingGroupMembership)(nil)
)

// ThingGroupMembership serves AWS::IoT::ThingGroupMembership: one thing in one static thing group. Its identifier is
// ThingGroupName and ThingName joined with "|". AWS has no call that changes a membership, so both properties are
// create-only, and UCP rejects a change with 400 before any AWS call.
type ThingGroupMembership struct {
	api API
}

// NewThingGroupMembership returns the handler for IoT thing group memberships.
func NewThingGroupMembership(api API) *ThingGroupMembership {
	return &ThingGroupMembership{api: api}
}

// TypeName returns AWS::IoT::ThingGroupMembership.
func (h *ThingGroupMembership) TypeName() string {
	return membershipType
}

// Options returns how UCP uses the handler. AWS lists memberships only by thing or by group, so the type can't be
// listed.
func (h *ThingGroupMembership) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// membershipID names a membership.
type membershipID struct {
	group, thing string
}

func (id membershipID) String() string {
	return id.group + "|" + id.thing
}

// clientFailure is a failure the template caused, which AWS never saw.
func clientFailure(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// parse returns the membership the properties name. ok is false when they don't name both the group and the thing.
// A value that isn't a string, or has "|", which separates the names in the identifier, is a client failure.
func parse(properties map[string]any) (id membershipID, ok bool, err error) {
	for _, field := range []struct {
		name  string
		value *string
	}{{"ThingGroupName", &id.group}, {"ThingName", &id.thing}} {
		value, set := properties[field.name]
		if !set || value == nil {
			continue
		}
		s, isString := value.(string)
		if !isString {
			return membershipID{}, false, clientFailure("%s of an AWS::IoT::ThingGroupMembership must be a string", field.name)
		}
		if strings.Contains(s, "|") {
			return membershipID{}, false, clientFailure("%s of an AWS::IoT::ThingGroupMembership can't contain \"|\", which Radius uses to join the names into the membership's identifier", field.name)
		}
		*field.value = s
	}
	return id, id.group != "" && id.thing != "", nil
}

// parseID returns the membership an identifier names.
func parseID(identifier string) (membershipID, error) {
	group, thing, ok := strings.Cut(identifier, "|")
	if !ok || group == "" || thing == "" || strings.Contains(thing, "|") {
		return membershipID{}, clientFailure("%q is not the identifier of an AWS IoT thing group membership: it must be ThingGroupName|ThingName", identifier)
	}
	return membershipID{group: group, thing: thing}, nil
}

// Identifier returns ThingGroupName|ThingName, or "" when the properties don't give both. UCP calls it before any AWS
// call, so a value of the wrong type is rejected here.
func (h *ThingGroupMembership) Identifier(properties map[string]any) (string, error) {
	id, ok, err := parse(properties)
	if !ok || err != nil {
		return "", err
	}
	return id.String(), nil
}

// member returns the membership an identifier names, and whether AWS lists the thing in the group. It returns
// ErrNotFound when the thing doesn't exist.
func (h *ThingGroupMembership) member(ctx context.Context, region string, identifier string) (membershipID, bool, error) {
	id, err := parseID(identifier)
	if err != nil {
		return membershipID{}, false, err
	}
	input := &iot.ListThingGroupsForThingInput{ThingName: aws.String(id.thing)}
	for {
		page, err := h.api.ListThingGroupsForThing(ctx, input, inRegion(region))
		if err != nil {
			return membershipID{}, false, awsapi.NotFound(err, "ResourceNotFoundException")
		}
		for _, group := range page.ThingGroups {
			if aws.ToString(group.GroupName) == id.group {
				return id, true, nil
			}
		}
		if input.NextToken = page.NextToken; input.NextToken == nil {
			return id, false, nil
		}
	}
}

// Read returns the membership's properties, or ErrNotFound when the thing isn't in the group or doesn't exist.
func (h *ThingGroupMembership) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, member, err := h.member(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, native.ErrNotFound
	}
	return map[string]any{"ThingGroupName": id.group, "ThingName": id.thing}, nil
}

// Create adds the thing to the group.
func (h *ThingGroupMembership) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, ok, err := parse(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", clientFailure("an AWS::IoT::ThingGroupMembership needs ThingGroupName and ThingName")
	}
	_, err = h.api.AddThingToThingGroup(ctx, &iot.AddThingToThingGroupInput{
		ThingGroupName: aws.String(id.group),
		ThingName:      aws.String(id.thing),
	}, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// Update changes nothing, because every property is create-only.
func (h *ThingGroupMembership) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete removes the thing from the group. It returns ErrNotFound, without calling AWS to remove it, when the thing
// is no longer in the group.
func (h *ThingGroupMembership) Delete(ctx context.Context, region string, identifier string) error {
	id, member, err := h.member(ctx, region, identifier)
	if err != nil {
		return err
	}
	if !member {
		return native.ErrNotFound
	}
	_, err = h.api.RemoveThingFromThingGroup(ctx, &iot.RemoveThingFromThingGroupInput{
		ThingGroupName: aws.String(id.group),
		ThingName:      aws.String(id.thing),
	}, inRegion(region))
	return awsapi.NotFound(err, "ResourceNotFoundException")
}

// List returns nothing, because AWS lists memberships only by thing or by group.
func (h *ThingGroupMembership) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads whether AWS lists the thing in the group. AWS adds and removes a thing in the call, so a create is
// done once AWS lists the thing in the group, and a delete once it doesn't, which is ErrNotFound. An update changes
// nothing, so it is done.
func (h *ThingGroupMembership) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	if _, err := h.Read(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "in the group", Waiting: "removed"}, nil
	}
	return native.Progress{Done: true}, nil
}
