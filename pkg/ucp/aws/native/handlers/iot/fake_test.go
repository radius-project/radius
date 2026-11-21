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

package iot

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iot"
	"github.com/aws/aws-sdk-go-v2/service/iot/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	arnPrefix = "arn:aws:iot:us-west-2:123456789012:"

	// thingType and groupType keep the fake's things and thing groups in the store, apart from the memberships.
	thingType = "AWS::IoT::Thing"
	groupType = "AWS::IoT::ThingGroup"

	// maxGroups is the most thing groups a thing can be in.
	maxGroups = 10

	// pageSize is how many groups a ListThingGroupsForThing page has when the call sets no maxResults. AWS doesn't
	// document it.
	pageSize = 2

	// invalidRequest is the error each call lists for a request that is not valid, which the fake returns for every
	// rule the call breaks, except a thing or group that doesn't exist.
	invalidRequest = "InvalidRequestException"
	notFound       = "ResourceNotFoundException"
)

// namePattern is the documented pattern of thing and thing group names.
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9:_-]{1,128}$`)

// groups are the fake's thing groups, by name, with their parent: fleet has the children fleet-east and fleet-west,
// labs and depot are other hierarchies, and online is a dynamic group, which has a query instead of members.
var groups = map[string]map[string]any{
	"fleet":      {"ThingGroupName": "fleet"},
	"fleet-east": {"ThingGroupName": "fleet-east", "ParentGroupName": "fleet"},
	"fleet-west": {"ThingGroupName": "fleet-west", "ParentGroupName": "fleet"},
	"labs":       {"ThingGroupName": "labs"},
	"depot":      {"ThingGroupName": "depot"},
	"online":     {"ThingGroupName": "online", "QueryString": "connectivity.connected:true"},
}

// things are the fake's things.
var things = []string{"sensor-1", "sensor-2", "sensor-3"}

// addRegistry adds the fake's things and thing groups to store.
func addRegistry(store *awsfake.CloudControl) {
	for name, group := range groups {
		store.Put(groupType, name, group)
	}
	for _, name := range things {
		store.Put(thingType, name, map[string]any{"ThingName": name})
	}
}

// fakeAPI is an in-memory AWS IoT API for thing group memberships. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. Things and thing groups are in the store
// too, under thingType and groupType. It follows the rules in membershipAPI.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// removes counts the RemoveThingFromThingGroup calls that reached the fake.
	removes int

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	addRegistry(store)
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// nameOf returns the name a call gives by name, or by ARN, whose resource part is kind/name, or "" when it gives
// neither.
func nameOf(name *string, arn *string, kind string) string {
	if name != nil {
		return *name
	}
	_, after, _ := strings.Cut(aws.ToString(arn), ":"+kind+"/")
	return after
}

// resolve returns the thing group and thing a call names, after checking that it names both with valid names and
// that both exist.
func (f *fakeAPI) resolve(groupName, groupArn, thingName, thingArn *string) (string, string, error) {
	group, thing := nameOf(groupName, groupArn, "thinggroup"), nameOf(thingName, thingArn, "thing")
	if !namePattern.MatchString(group) || !namePattern.MatchString(thing) {
		return "", "", fault(invalidRequest)
	}
	if f.store.Resource(groupType, group) == nil || f.store.Resource(thingType, thing) == nil {
		return "", "", fault(notFound)
	}
	return group, thing, nil
}

// root returns the top group of the hierarchy group is in.
func (f *fakeAPI) root(group string) string {
	for {
		parent, _ := f.store.Resource(groupType, group)["ParentGroupName"].(string)
		if parent == "" {
			return group
		}
		group = parent
	}
}

// member reports whether the thing is in the group.
func (f *fakeAPI) member(group string, thing string) bool {
	return f.store.Resource(membershipType, group+"|"+thing) != nil
}

// groupsOf returns the groups the thing is in, in the order the store lists them.
func (f *fakeAPI) groupsOf(thing string) []string {
	out := []string{}
	for _, name := range f.store.List(groupType) {
		if f.member(name, thing) {
			out = append(out, name)
		}
	}
	return out
}

func (f *fakeAPI) AddThingToThingGroup(ctx context.Context, in *iot.AddThingToThingGroupInput, _ ...func(*iot.Options)) (*iot.AddThingToThingGroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["AddThingToThingGroup"]; err != nil {
		return nil, err
	}
	group, thing, err := f.resolve(in.ThingGroupName, in.ThingGroupArn, in.ThingName, in.ThingArn)
	if err != nil {
		return nil, err
	}
	if f.member(group, thing) {
		return &iot.AddThingToThingGroupOutput{}, nil
	}
	if f.store.Resource(groupType, group)["QueryString"] != nil {
		return nil, fault(invalidRequest)
	}
	count := 0
	for _, other := range f.store.List(groupType) {
		if !f.member(other, thing) {
			continue
		}
		count++
		if f.root(other) == f.root(group) {
			return nil, fault(invalidRequest)
		}
	}
	if count >= maxGroups {
		return nil, fault(invalidRequest)
	}
	id := group + "|" + thing
	f.store.Put(membershipType, id, map[string]any{"ThingGroupName": group, "ThingName": thing})
	return &iot.AddThingToThingGroupOutput{}, nil
}

func (f *fakeAPI) RemoveThingFromThingGroup(ctx context.Context, in *iot.RemoveThingFromThingGroupInput, _ ...func(*iot.Options)) (*iot.RemoveThingFromThingGroupOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	if err := f.errs["RemoveThingFromThingGroup"]; err != nil {
		return nil, err
	}
	group, thing, err := f.resolve(in.ThingGroupName, in.ThingGroupArn, in.ThingName, in.ThingArn)
	if err != nil {
		return nil, err
	}
	f.store.Remove(membershipType, group+"|"+thing)
	return &iot.RemoveThingFromThingGroupOutput{}, nil
}

// ListThingGroupsForThing returns maxResults groups per page, or pageSize when the call sets none.
func (f *fakeAPI) ListThingGroupsForThing(ctx context.Context, in *iot.ListThingGroupsForThingInput, _ ...func(*iot.Options)) (*iot.ListThingGroupsForThingOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListThingGroupsForThing"]; err != nil {
		return nil, err
	}
	thing := aws.ToString(in.ThingName)
	size := pageSize
	if in.MaxResults != nil {
		size = int(*in.MaxResults)
	}
	start, err := strconv.Atoi(aws.ToString(in.NextToken))
	if !namePattern.MatchString(thing) || size < 1 || size > 250 || (in.NextToken != nil && (err != nil || start < 0)) {
		return nil, fault(invalidRequest)
	}
	if f.store.Resource(thingType, thing) == nil {
		return nil, fault(notFound)
	}
	names := f.groupsOf(thing)
	end := min(start+size, len(names))
	out := &iot.ListThingGroupsForThingOutput{ThingGroups: []types.GroupNameAndArn{}}
	for _, name := range names[min(start, end):end] {
		out.ThingGroups = append(out.ThingGroups, types.GroupNameAndArn{GroupName: aws.String(name), GroupArn: aws.String(arnPrefix + "thinggroup/" + name)})
	}
	if end < len(names) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	return out, nil
}
