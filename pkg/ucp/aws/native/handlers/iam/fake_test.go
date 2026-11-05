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

package iam

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// outsideType is the pseudo type the store keeps, per group, the users added outside Radius under.
	outsideType = "outside::" + userToGroupAdditionType

	// missingPrefix starts the names of groups and users AWS doesn't have.
	missingPrefix = "missing"

	// pageSize is how many users GetGroup returns per page.
	pageSize = 2
)

// fakeAPI is an in-memory IAM API for group members. The store keeps one entry per identifier, holding the users that
// alias added, and one entry per group under outsideType for users added outside Radius. A group's users are the
// users of all its entries. An add puts the user in the entry of the current identifier, which the tracked handler
// sets, and a removal takes the user out of every entry of the group, as IAM does. An entry with no users is removed.
// Every group and user exists, except those named with missingPrefix.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu      sync.Mutex
	current string
	calls   []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) setCurrent(identifier string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = identifier
}

// entry is an entry of the store.
type entry struct {
	typeName   string
	identifier string
	properties map[string]any
}

// entries returns the store's entries of the group.
func (f *fakeAPI) entries(group string) []entry {
	out := []entry{}
	for _, id := range f.store.List(userToGroupAdditionType) {
		if props := f.store.Resource(userToGroupAdditionType, id); awsapi.String(props, "GroupName") == group {
			out = append(out, entry{userToGroupAdditionType, id, props})
		}
	}
	if props := f.store.Resource(outsideType, group); props != nil {
		out = append(out, entry{outsideType, group, props})
	}
	return out
}

// users returns the group's users, once each, ignoring case, sorted.
func (f *fakeAPI) users(group string) []string {
	out := []string{}
	for _, e := range f.entries(group) {
		for _, u := range awsapi.Strings(e.properties, "Users") {
			if !containsFold(out, u) {
				out = append(out, u)
			}
		}
	}
	slices.Sort(out)
	return out
}

func containsFold(list []string, s string) bool {
	return slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, s) })
}

func (f *fakeAPI) put(e entry, users []string) {
	if len(users) == 0 {
		f.store.Remove(e.typeName, e.identifier)
		return
	}
	list := []any{}
	for _, u := range users {
		list = append(list, u)
	}
	e.properties["Users"] = list
	f.store.Put(e.typeName, e.identifier, e.properties)
}

// change makes a call that changes AWS, failing it as the failer says.
func (f *fakeAPI) change(method string, group string, user string, apply func() error) error {
	f.calls = append(f.calls, method)
	if err := f.errs[method]; err != nil {
		return err
	}
	if err := f.failer.Before(); err != nil {
		return err
	}
	if strings.HasPrefix(group, missingPrefix) || strings.HasPrefix(user, missingPrefix) {
		return fault("NoSuchEntity")
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

func (f *fakeAPI) GetGroup(ctx context.Context, in *iam.GetGroupInput, _ ...func(*iam.Options)) (*iam.GetGroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["GetGroup"]; err != nil {
		return nil, err
	}
	group := aws.ToString(in.GroupName)
	if strings.HasPrefix(group, missingPrefix) {
		return nil, fault("NoSuchEntity")
	}
	users := f.users(group)
	start, _ := strconv.Atoi(aws.ToString(in.Marker))
	end := min(start+pageSize, len(users))
	out := &iam.GetGroupOutput{Group: &types.Group{GroupName: aws.String(group)}}
	for _, u := range users[start:end] {
		out.Users = append(out.Users, types.User{UserName: aws.String(u)})
	}
	if end < len(users) {
		out.IsTruncated, out.Marker = true, aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) AddUserToGroup(ctx context.Context, in *iam.AddUserToGroupInput, _ ...func(*iam.Options)) (*iam.AddUserToGroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	group, user := aws.ToString(in.GroupName), aws.ToString(in.UserName)
	return &iam.AddUserToGroupOutput{}, f.change("AddUserToGroup", group, user, func() error {
		identifier := f.current
		if identifier == "" {
			identifier = group
		}
		props := f.store.Resource(userToGroupAdditionType, identifier)
		if props == nil {
			props = map[string]any{"Id": identifier, "GroupName": group}
		}
		users := awsapi.Strings(props, "Users")
		if !containsFold(users, user) {
			users = append(users, user)
		}
		f.put(entry{userToGroupAdditionType, identifier, props}, users)
		return nil
	})
}

func (f *fakeAPI) RemoveUserFromGroup(ctx context.Context, in *iam.RemoveUserFromGroupInput, _ ...func(*iam.Options)) (*iam.RemoveUserFromGroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	group, user := aws.ToString(in.GroupName), aws.ToString(in.UserName)
	return &iam.RemoveUserFromGroupOutput{}, f.change("RemoveUserFromGroup", group, user, func() error {
		if !containsFold(f.users(group), user) {
			return fault("NoSuchEntity")
		}
		for _, e := range f.entries(group) {
			users := slices.DeleteFunc(awsapi.Strings(e.properties, "Users"), func(u string) bool { return strings.EqualFold(u, user) })
			f.put(e, users)
		}
		return nil
	})
}

// tracked is the handler with its fake API, which it tells the identifier of each call, so an add puts the user in
// that identifier's entry.
type tracked struct {
	*UserToGroupAddition
	api *fakeAPI
}

func newTracked(store *awsfake.CloudControl) *tracked {
	api := newFakeAPI(store)
	return &tracked{UserToGroupAddition: NewUserToGroupAddition(api), api: api}
}

func (t *tracked) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	t.api.setCurrent(awsapi.String(request.Properties, "GroupName") + "|" + native.GenerateName(request.ClientToken))
	return t.UserToGroupAddition.Create(ctx, request)
}

func (t *tracked) Update(ctx context.Context, request native.UpdateRequest) error {
	t.api.setCurrent(request.Identifier)
	return t.UserToGroupAddition.Update(ctx, request)
}

func (t *tracked) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	t.api.setCurrent(identifier)
	return t.UserToGroupAddition.ReadScoped(ctx, region, identifier, managed)
}

func (t *tracked) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	t.api.setCurrent(identifier)
	return t.UserToGroupAddition.DeleteScoped(ctx, region, identifier, managed)
}
