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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iot"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/iot/latest/apireference/"
	groupsGuide  = "https://docs.aws.amazon.com/iot/latest/developerguide/thing-groups.html"

	// constraintNote is what every call's page leaves unclear about its constraints.
	constraintNote = " The page names no error for a name outside its length or pattern; the fake returns " +
		"InvalidRequestException, the only error the call lists for a request that is not valid."
)

// apiCall is a call against a fake IoT API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake IoT API backed by store. It stops
// at the first call that fails, and returns its error.
func against(calls ...apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// add adds the thing to the group, by name.
func add(group string, thing string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.AddThingToThingGroup(ctx, &iot.AddThingToThingGroupInput{ThingGroupName: aws.String(group), ThingName: aws.String(thing)})
		return err
	}
}

// remove removes the thing from the group, by name.
func remove(group string, thing string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.RemoveThingFromThingGroup(ctx, &iot.RemoveThingFromThingGroupInput{ThingGroupName: aws.String(group), ThingName: aws.String(thing)})
		return err
	}
}

// listGroups returns the names of the groups AWS lists the thing in, from every page.
func listGroups(ctx context.Context, f *fakeAPI, in *iot.ListThingGroupsForThingInput) ([]string, error) {
	out := []string{}
	for {
		page, err := f.ListThingGroupsForThing(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, g := range page.ThingGroups {
			out = append(out, aws.ToString(g.GroupName))
		}
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return out, nil
		}
	}
}

// expectGroups returns a call that fails unless AWS lists the thing in exactly want, in any order.
func expectGroups(thing string, want ...string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		got, err := listGroups(ctx, f, &iot.ListThingGroupsForThingInput{ThingName: aws.String(thing)})
		if err != nil {
			return err
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			return fmt.Errorf("%s is in %v, not %v", thing, got, want)
		}
		return nil
	}
}

// addRoots adds n thing groups, each the top of its own hierarchy, named root-0 and on.
func addRoots(n int) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		for i := range n {
			name := "root-" + strconv.Itoa(i)
			f.store.Put(groupType, name, map[string]any{"ThingGroupName": name})
		}
		return nil
	}
}

// addToRoots adds the thing to the n groups addRoots adds.
func addToRoots(thing string, n int) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		for i := range n {
			if err := add("root-"+strconv.Itoa(i), thing)(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// membershipAPI records the IoT calls ThingGroupMembership makes.
var membershipAPI = map[string]nativetest.Call{
	"AddThingToThingGroup": {
		Doc: apiReference + "API_AddThingToThingGroup.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "thingName is 1 to 128 characters of a-z, A-Z, 0-9, colon, underscore, and hyphen",
				Break: against(add("fleet-east", "sensor 1")),
				Code:  invalidRequest,
			},
			{
				Rule:  "thingGroupName is at most 128 characters",
				Break: against(add(strings.Repeat("g", 129), "sensor-1")),
				Code:  invalidRequest,
			},
			{
				Rule:  "the thing exists",
				Break: against(add("fleet-east", "missing")),
				Code:  notFound,
			},
			{
				Rule:  "the thing group exists",
				Break: against(add("missing", "sensor-1")),
				Code:  notFound,
			},
			{
				Rule:  "a thing can be in at most 10 thing groups",
				Doc:   groupsGuide,
				Break: against(addRoots(11), addToRoots("sensor-1", 11)),
				Code:  invalidRequest,
			},
			{
				Rule:  "a thing can't be in two groups of the same hierarchy, such as two children of one parent",
				Doc:   groupsGuide,
				Break: against(add("fleet-east", "sensor-1"), add("fleet-west", "sensor-1")),
				Code:  invalidRequest,
			},
			{
				Rule:  "a thing can't be in a group and its parent, which are in the same hierarchy",
				Doc:   groupsGuide,
				Break: against(add("fleet", "sensor-1"), add("fleet-east", "sensor-1")),
				Code:  invalidRequest,
			},
			{
				Rule:  "a thing can be in groups of different hierarchies",
				Doc:   groupsGuide,
				Check: against(add("fleet-east", "sensor-1"), add("labs", "sensor-1"), expectGroups("sensor-1", "fleet-east", "labs")),
			},
			{
				Rule:  "a thing can be in 10 thing groups",
				Doc:   groupsGuide,
				Check: against(addRoots(10), addToRoots("sensor-1", 10)),
			},
			{
				Rule: "thingArn and thingGroupArn name the thing and the group instead of their names",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.AddThingToThingGroup(ctx, &iot.AddThingToThingGroupInput{
						ThingGroupArn: aws.String(arnPrefix + "thinggroup/labs"),
						ThingArn:      aws.String(arnPrefix + "thing/sensor-2"),
					})
					return err
				}, expectGroups("sensor-2", "labs")),
			},
		},
		Unclear: "The page doesn't say what AWS does when the thing is already in the group; the fake changes nothing " +
			"and succeeds. The handler reads the membership before it creates one, so it doesn't rely on it. The guide " +
			"limits a thing to 10 groups and to one group per hierarchy without naming the error; the fake returns " +
			"InvalidRequestException. The docs don't say whether a thing can be added to a dynamic thing group, whose " +
			"members AWS picks by a query; the fake rejects it with InvalidRequestException. The handler never sets " +
			"overrideDynamicGroups, which can remove the thing from a dynamic group, and the fake ignores it." + constraintNote,
	},
	"RemoveThingFromThingGroup": {
		Doc: apiReference + "API_RemoveThingFromThingGroup.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the call names the group by thingGroupArn or thingGroupName",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.RemoveThingFromThingGroup(ctx, &iot.RemoveThingFromThingGroupInput{ThingName: aws.String("sensor-1")})
					return err
				}),
				Code: invalidRequest,
			},
			{
				Rule:  "the thing exists",
				Break: against(remove("fleet-east", "missing")),
				Code:  notFound,
			},
			{
				Rule:  "the thing group exists",
				Break: against(remove("missing", "sensor-1")),
				Code:  notFound,
			},
			{
				Rule:  "a removed thing is no longer in the group, and stays in its other groups",
				Check: against(add("fleet-east", "sensor-1"), add("labs", "sensor-1"), remove("fleet-east", "sensor-1"), expectGroups("sensor-1", "labs")),
			},
			{
				Rule:  "once removed, the thing can join another group of the same hierarchy",
				Doc:   groupsGuide,
				Check: against(add("fleet-east", "sensor-1"), remove("fleet-east", "sensor-1"), add("fleet-west", "sensor-1")),
			},
		},
		Unclear: "The page doesn't say what AWS does when the thing isn't in the group; the fake changes nothing and " +
			"succeeds. The handler checks that the thing is in the group first, so it doesn't rely on it." + constraintNote,
	},
	"ListThingGroupsForThing": {
		Doc: apiReference + "API_ListThingGroupsForThing.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the thing exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListThingGroupsForThing(ctx, &iot.ListThingGroupsForThingInput{ThingName: aws.String("missing")})
					return err
				}),
				Code: notFound,
			},
			{
				Rule: "maxResults is 1 to 250",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListThingGroupsForThing(ctx, &iot.ListThingGroupsForThingInput{ThingName: aws.String("sensor-1"), MaxResults: aws.Int32(251)})
					return err
				}),
				Code: invalidRequest,
			},
			{
				Rule: "each group has its name and ARN, and nextToken reads the next page",
				Check: against(add("fleet-east", "sensor-3"), add("labs", "sensor-3"), add("depot", "sensor-3"), func(ctx context.Context, f *fakeAPI) error {
					page, err := f.ListThingGroupsForThing(ctx, &iot.ListThingGroupsForThingInput{ThingName: aws.String("sensor-3"), MaxResults: aws.Int32(1)})
					if err != nil {
						return err
					}
					if len(page.ThingGroups) != 1 || page.NextToken == nil || !strings.HasSuffix(aws.ToString(page.ThingGroups[0].GroupArn), ":thinggroup/"+aws.ToString(page.ThingGroups[0].GroupName)) {
						return fmt.Errorf("the first page is %+v", page)
					}
					return expectGroups("sensor-3", "fleet-east", "labs", "depot")(ctx, f)
				}),
			},
			{
				Rule:  "a thing in no group has an empty list",
				Check: against(expectGroups("sensor-2")),
			},
		},
		Unclear: "The page doesn't say whether it lists a group's parent groups, or the dynamic groups AWS put the thing " +
			"in; the fake lists only the groups the thing was added to. It doesn't give a page size or say how soon a " +
			"change shows; the fake returns 2 groups a page, and shows a change at once." + constraintNote,
	},
}
