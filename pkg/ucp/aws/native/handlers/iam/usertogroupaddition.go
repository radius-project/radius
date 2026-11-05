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

// Package iam has the native handler for adding IAM users to a group (AWS::IAM::UserToGroupAddition), a type Cloud
// Control can't create in any region.
package iam

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const userToGroupAdditionType = "AWS::IAM::UserToGroupAddition"

// API is the part of the IAM client the handler uses.
type API interface {
	GetGroup(ctx context.Context, params *iam.GetGroupInput, optFns ...func(*iam.Options)) (*iam.GetGroupOutput, error)
	AddUserToGroup(ctx context.Context, params *iam.AddUserToGroupInput, optFns ...func(*iam.Options)) (*iam.AddUserToGroupOutput, error)
	RemoveUserFromGroup(ctx context.Context, params *iam.RemoveUserFromGroupInput, optFns ...func(*iam.Options)) (*iam.RemoveUserFromGroupOutput, error)
}

func inRegion(region string) func(*iam.Options) {
	return func(o *iam.Options) { o.Region = region }
}

var (
	_ native.Handler       = (*UserToGroupAddition)(nil)
	_ native.ScopedReader  = (*UserToGroupAddition)(nil)
	_ native.ScopedDeleter = (*UserToGroupAddition)(nil)
)

// UserToGroupAddition serves AWS::IAM::UserToGroupAddition: the users in Users, as members of the group GroupName.
// Others can add users to the same group, so the handler reads, changes, and removes only the users the template
// added, and leaves the others in the group. Its identifier is GroupName and a name UCP generates from the record's
// client token, joined with "|", so several templates can add users to one group. Adding a user who is already in
// the group changes nothing, so UCP finds a lost create by sending it again. IAM changes users one call at a time, and
// every call takes effect at once. IAM names don't differ only by case, so users are matched ignoring case.
type UserToGroupAddition struct {
	api API
}

// NewUserToGroupAddition returns the handler for adding IAM users to a group.
func NewUserToGroupAddition(api API) *UserToGroupAddition {
	return &UserToGroupAddition{api: api}
}

// TypeName returns AWS::IAM::UserToGroupAddition.
func (h *UserToGroupAddition) TypeName() string {
	return userToGroupAdditionType
}

// Options returns how UCP uses the handler.
func (h *UserToGroupAddition) Options() native.Options {
	return native.Options{
		List:     native.ListNone,
		Recovery: native.RecoverByResend,
	}
}

// groupOf returns the group name in identifier. An identifier without one, such as one another handler assigned, takes
// GroupName from the first of properties that sets it.
func groupOf(identifier string, properties ...map[string]any) (string, error) {
	if group, _, ok := strings.Cut(identifier, "|"); ok && group != "" {
		return group, nil
	}
	for _, p := range properties {
		if group := awsapi.String(p, "GroupName"); group != "" {
			return group, nil
		}
	}
	return "", &native.Failure{
		Message:     fmt.Sprintf("the identifier %q of the AWS type %s names no group", identifier, userToGroupAdditionType),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// members returns the names of the group's users, or ErrNotFound when AWS has no such group.
func (h *UserToGroupAddition) members(ctx context.Context, region string, group string) ([]string, error) {
	out := []string{}
	var marker *string
	for {
		page, err := h.api.GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String(group), Marker: marker}, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "NoSuchEntity")
		}
		for _, u := range page.Users {
			out = append(out, aws.ToString(u.UserName))
		}
		if !page.IsTruncated || page.Marker == nil {
			return out, nil
		}
		marker = page.Marker
	}
}

// spellings returns the users the property sets list, by their names in lowercase, each with the last spelling a
// set uses.
func spellings(sets ...map[string]any) map[string]string {
	out := map[string]string{}
	for _, p := range sets {
		for _, u := range awsapi.Strings(p, "Users") {
			out[strings.ToLower(u)] = u
		}
	}
	return out
}

// without returns the users of from that aren't in the users of other, ignoring case, once each.
func without(from map[string]any, other map[string]any) []string {
	skip := spellings(other)
	out := []string{}
	for _, u := range awsapi.Strings(from, "Users") {
		key := strings.ToLower(u)
		if _, ok := skip[key]; !ok {
			out = append(out, u)
			skip[key] = u
		}
	}
	return out
}

// Read returns every user of the group.
func (h *UserToGroupAddition) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.ReadScoped(ctx, region, identifier, nil)
}

// ReadScoped returns the users of the group that managed lists, with the template's spelling, or every user when
// managed is nil. It returns ErrNotFound when the group has none of them, or AWS has no such group.
func (h *UserToGroupAddition) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	group, users, err := h.managedUsers(ctx, region, identifier, managed)
	if err != nil {
		return nil, err
	}
	list := []any{}
	for _, u := range users {
		list = append(list, u)
	}
	return map[string]any{"Id": identifier, "GroupName": group, "Users": list}, nil
}

// managedUsers returns the group of the identifier and its users that managed lists, as ReadScoped does.
func (h *UserToGroupAddition) managedUsers(ctx context.Context, region string, identifier string, managed []map[string]any) (string, []string, error) {
	group, err := groupOf(identifier, managed...)
	if err != nil {
		return "", nil, err
	}
	present, err := h.members(ctx, region, group)
	if err != nil {
		return "", nil, err
	}
	users := present
	if managed != nil {
		names := spellings(managed...)
		users = []string{}
		for _, u := range present {
			if name, ok := names[strings.ToLower(u)]; ok {
				users = append(users, name)
			}
		}
	}
	if len(users) == 0 {
		return "", nil, native.ErrNotFound
	}
	return group, users, nil
}

func (h *UserToGroupAddition) add(ctx context.Context, region string, group string, user string) func() error {
	return func() error {
		_, err := h.api.AddUserToGroup(ctx, &iam.AddUserToGroupInput{GroupName: aws.String(group), UserName: aws.String(user)}, inRegion(region))
		return err
	}
}

// remove returns a call that removes the user from the group. A user who is no longer in the group is already
// removed.
func (h *UserToGroupAddition) remove(ctx context.Context, region string, group string, user string) func() error {
	return func() error {
		_, err := h.api.RemoveUserFromGroup(ctx, &iam.RemoveUserFromGroupInput{GroupName: aws.String(group), UserName: aws.String(user)}, inRegion(region))
		if awsapi.HasCode(err, "NoSuchEntity") {
			return nil
		}
		return err
	}
}

// Create adds the users to the group, one call each, and returns the identifier. A failure after the first call is
// partly applied, and a redeploy adds the rest.
func (h *UserToGroupAddition) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	group := awsapi.String(request.Properties, "GroupName")
	identifier := group + "|" + native.GenerateName(request.ClientToken)
	calls := []func() error{}
	for _, u := range without(request.Properties, nil) {
		calls = append(calls, h.add(ctx, request.Region, group, u))
	}
	return identifier, awsapi.Calls(calls...)
}

// Update removes the users the template no longer lists, then adds the new ones, one call each. A failure while it
// removes users is rejected, so the record keeps every user it may still have to remove. A failure while it adds
// users after an earlier call is partly applied.
func (h *UserToGroupAddition) Update(ctx context.Context, request native.UpdateRequest) error {
	group, err := groupOf(request.Identifier, request.Desired, request.Current)
	if err != nil {
		return err
	}
	calls := []func() error{}
	for _, u := range without(request.Current, request.Desired) {
		calls = append(calls, rejected(h.remove(ctx, request.Region, group, u)))
	}
	for _, u := range without(request.Desired, request.Current) {
		calls = append(calls, h.add(ctx, request.Region, group, u))
	}
	return awsapi.Calls(calls...)
}

// rejected returns call, with its error as a failure AWS applied none of, even when the response was lost.
func rejected(call func() error) func() error {
	return func() error {
		if err := call(); err != nil {
			return &native.Failure{Message: err.Error(), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
		}
		return nil
	}
}

// Delete fails, because a delete removes only the users the template added. UCP calls DeleteScoped instead.
func (h *UserToGroupAddition) Delete(ctx context.Context, region string, identifier string) error {
	return fmt.Errorf("the native handler for %s removes only the users Radius manages, with DeleteScoped", userToGroupAdditionType)
}

// DeleteScoped removes the users managed lists that are still in the group, one call each. It returns ErrNotFound
// when none are there, or AWS has no such group. Unlike ReadScoped, a nil managed lists no users.
func (h *UserToGroupAddition) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	if managed == nil {
		managed = []map[string]any{}
	}
	group, users, err := h.managedUsers(ctx, region, identifier, managed)
	if err != nil {
		return err
	}
	calls := []func() error{}
	for _, u := range users {
		calls = append(calls, h.remove(ctx, region, group, u))
	}
	return awsapi.Calls(calls...)
}

// List fails, because users added to groups can't be listed by the identifiers UCP generates.
func (h *UserToGroupAddition) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", userToGroupAdditionType)
}

// Progress reports every operation as done, because adding and removing users takes effect at once.
func (h *UserToGroupAddition) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
