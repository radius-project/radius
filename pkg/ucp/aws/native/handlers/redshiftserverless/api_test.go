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

package redshiftserverless

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/redshift-serverless/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	domainGuide  = "https://docs.aws.amazon.com/redshift/latest/mgmt/connecting-connection-CNAME-create-custom-domain.html"

	// listUnclear is what the list pages leave unclear.
	listUnclear = "The page doesn't say what nextToken looks like; the fake's tokens are 8 or more characters, as the page " +
		"requires, and it answers one it didn't make with InvalidPaginationException."

	// busyUnclear is what the pages leave unclear about a busy workgroup.
	busyUnclear = " The page names ConflictException without saying when AWS returns it. The handler answers it with 409, " +
		"so the caller deploys again, and names AWS's message."

	testCertificate = "arn:aws:acm:us-west-2:" + account + ":certificate/11111111-1111-1111-1111-111111111111"
)

// apiCall is a call against a fake Redshift Serverless API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake API backed by store, which has the
// fake's workgroups. It stops at the first call that fails, and returns its error.
func against(calls ...apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		addWorkgroups(store)
		f := newFakeAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// hold returns a call that keeps the workgroup with the name MODIFYING.
func hold(name string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		f.held[name] = true
		return nil
	}
}

// expectWorkgroup returns a call that reads the workgroup analytics and fails unless its status is want.
func expectWorkgroup(want types.WorkgroupStatus) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		out, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String("analytics")})
		if err != nil {
			return err
		}
		if out.Workgroup.Status != want {
			return fmt.Errorf("the workgroup is %s, not %s", out.Workgroup.Status, want)
		}
		return nil
	}
}

// settle reads the workgroup analytics until it is free.
func settle(ctx context.Context, f *fakeAPI) error {
	for range busyReads {
		if _, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String("analytics")}); err != nil {
			return err
		}
	}
	return nil
}

// pagingRules returns the rules of a list call's maxResults and pages. list makes the call with token and maxResults,
// after put stores n items, and returns how many the page has and its token.
func pagingRules(put func(store *awsfake.CloudControl, n int), list func(ctx context.Context, f *fakeAPI, token *string, maxResults *int32) (int, *string, error)) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "maxResults is 1 to 100",
			Break: against(func(ctx context.Context, f *fakeAPI) error {
				_, _, err := list(ctx, f, nil, aws.Int32(101))
				return err
			}),
			Code: validation,
		},
		{
			Rule: "a nextToken the list didn't return is invalid",
			Break: against(func(ctx context.Context, f *fakeAPI) error {
				_, _, err := list(ctx, f, aws.String("not-a-token"), nil)
				return err
			}),
			Code: "InvalidPaginationException",
		},
		{
			Rule: "a call without maxResults returns up to 100 items, and a nextToken for the rest",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				addWorkgroups(store)
				put(store, 101)
				f := newFakeAPI(store)
				n, token, err := list(ctx, f, nil, nil)
				if err != nil {
					return err
				}
				if n != 100 || token == nil || len(*token) < 8 {
					return fmt.Errorf("the first page has %d items and token %v", n, token)
				}
				if n, token, err = list(ctx, f, token, nil); err != nil || n != 1 || token != nil {
					return fmt.Errorf("the second page has %d items and token %v: %v", n, token, err)
				}
				return nil
			},
		},
	}
}

// createLimit creates a serverless-compute usage limit of 10 RPU hours on the workgroup analytics, with the changes
// edits make to the input.
func createLimit(edits ...func(*redshiftserverless.CreateUsageLimitInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &redshiftserverless.CreateUsageLimitInput{
			ResourceArn: aws.String(workgroupArn("analytics")),
			UsageType:   types.UsageLimitUsageTypeServerlessCompute,
			Amount:      aws.Int64(10),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateUsageLimit(ctx, in)
		return err
	}
}

// firstLimit is the ID the fake gives its first usage limit.
const firstLimit = "00000001-0000-4000-8000-000000000000"

func getLimit(id string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.GetUsageLimit(ctx, &redshiftserverless.GetUsageLimitInput{UsageLimitId: aws.String(id)})
		return err
	}
}

func updateLimit(in *redshiftserverless.UpdateUsageLimitInput) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		if in.UsageLimitId == nil {
			in.UsageLimitId = aws.String(firstLimit)
		}
		_, err := f.UpdateUsageLimit(ctx, in)
		return err
	}
}

func deleteLimit(id string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.DeleteUsageLimit(ctx, &redshiftserverless.DeleteUsageLimitInput{UsageLimitId: aws.String(id)})
		return err
	}
}

// putLimits stores n usage limits of the workgroup analytics.
func putLimits(store *awsfake.CloudControl, n int) {
	for i := range n {
		id := strconv.Itoa(i)
		store.Put(usageLimitType, id, map[string]any{
			"UsageLimitId": id, "ResourceArn": workgroupArn("analytics"), "UsageType": "serverless-compute",
			"Amount": float64(1), "Period": "daily", "BreachAction": "log",
		})
	}
}

// usageLimitAPI records the Redshift Serverless calls UsageLimit makes.
var usageLimitAPI = map[string]nativetest.Call{
	"CreateUsageLimit": {
		Doc: apiReference + "API_CreateUsageLimit.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "amount is a positive number",
				Break: against(createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.Amount = aws.Int64(0) })),
				Code:  validation,
			},
			{
				Rule:  "usageType is serverless-compute or cross-region-datasharing",
				Break: against(createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.UsageType = "compute" })),
				Code:  validation,
			},
			{
				Rule:  "period is daily, weekly, or monthly",
				Break: against(createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.Period = "yearly" })),
				Code:  validation,
			},
			{
				Rule:  "breachAction is log, emit-metric, or deactivate",
				Break: against(createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.BreachAction = "stop" })),
				Code:  validation,
			},
			{
				Rule:  "resourceArn is a workgroup's ARN, not a namespace's or one of no workgroup",
				Break: against(createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.ResourceArn = aws.String(namespaceArn) })),
				Code:  validation,
			},
			{
				Rule: "a workgroup has at most 4 usage limits of one usage type",
				Break: against(createLimit(), createLimit(), createLimit(), createLimit(),
					createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.Amount = aws.Int64(20) })),
				Code: "ServiceQuotaExceededException",
			},
			{
				Rule: "the period is monthly and the breach action log when the create leaves them out",
				Check: against(createLimit(), func(ctx context.Context, f *fakeAPI) error {
					out, err := f.GetUsageLimit(ctx, &redshiftserverless.GetUsageLimitInput{UsageLimitId: aws.String(firstLimit)})
					if err != nil {
						return err
					}
					if out.UsageLimit.Period != types.UsageLimitPeriodMonthly || out.UsageLimit.BreachAction != types.UsageLimitBreachActionLog {
						return fmt.Errorf("the limit has period %s and breach action %s", out.UsageLimit.Period, out.UsageLimit.BreachAction)
					}
					return nil
				}),
			},
			{
				Rule:  "AWS accepts a second usage limit with the same values, and gives it another ID",
				Check: against(createLimit(), createLimit(), getLimit(firstLimit), getLimit("00000002-0000-4000-8000-000000000000")),
			},
			{
				Rule:  "a create is accepted while the workgroup is MODIFYING, and leaves it MODIFYING",
				Check: against(hold("analytics"), createLimit(), func(ctx context.Context, f *fakeAPI) error { delete(f.held, "analytics"); return nil }, expectWorkgroup(types.WorkgroupStatusModifying)),
			},
		},
		Unclear: "The page lists ValidationException and ServiceQuotaExceededException without the values or limits they " +
			"cover. AWS rejected a namespace's ARN and one of no workgroup with ValidationException, and a fifth limit of one " +
			"usage type with ServiceQuotaExceededException (\"Cannot create more than 4 usage limits\"), accepted a second " +
			"limit with the same values, accepted a create while the workgroup was MODIFYING, and left the workgroup and its " +
			"namespace MODIFYING for about 10 seconds after each create, update, and delete, in a run against real AWS. The " +
			"call takes no idempotency token, and TagResource rejects a usage limit's ARN, so the handler finds a lost create " +
			"by its values (RecoverBySearch) and fails when more than one limit has them.",
	},
	"GetUsageLimit": {
		Doc: apiReference + "API_GetUsageLimit.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "an ID AWS has no usage limit for is invalid",
				Break: against(getLimit("missing")),
				Code:  validation,
			},
		},
		Unclear: "The page lists ResourceNotFoundException, but AWS answered an ID it had no usage limit for, a deleted one " +
			"or one in another form, with ValidationException (\"does not exist\") in a run against real AWS. The ID is the " +
			"call's only input, so the handler takes both as not found. The page doesn't say how soon a deleted limit is " +
			"gone; the fake still returns it for one read.",
	},
	"UpdateUsageLimit": {
		Doc: apiReference + "API_UpdateUsageLimit.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the usage limit exists",
				Break: against(updateLimit(&redshiftserverless.UpdateUsageLimitInput{Amount: aws.Int64(5)})),
				Code:  notFound,
			},
			{
				Rule:  "amount is a positive number",
				Break: against(createLimit(), updateLimit(&redshiftserverless.UpdateUsageLimitInput{Amount: aws.Int64(-1)})),
				Code:  validation,
			},
			{
				Rule:  "breachAction is log, emit-metric, or deactivate",
				Break: against(createLimit(), updateLimit(&redshiftserverless.UpdateUsageLimitInput{BreachAction: "stop"})),
				Code:  validation,
			},
			{
				Rule: "an update changes only what it sends, and leaves the workgroup MODIFYING",
				Check: against(createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.BreachAction = "emit-metric" }), settle,
					updateLimit(&redshiftserverless.UpdateUsageLimitInput{Amount: aws.Int64(5)}), expectWorkgroup(types.WorkgroupStatusModifying),
					func(ctx context.Context, f *fakeAPI) error {
						out, err := f.GetUsageLimit(ctx, &redshiftserverless.GetUsageLimitInput{UsageLimitId: aws.String(firstLimit)})
						if err != nil {
							return err
						}
						if aws.ToInt64(out.UsageLimit.Amount) != 5 || out.UsageLimit.BreachAction != "emit-metric" {
							return fmt.Errorf("the limit has amount %d and breach action %s", aws.ToInt64(out.UsageLimit.Amount), out.UsageLimit.BreachAction)
						}
						return nil
					}),
			},
		},
		Unclear: "The page says the usage type and period can't change, and the call has no parameter for them. It doesn't " +
			"say whether an omitted breachAction keeps the current one; the fake keeps it, and the handler sends log, AWS's " +
			"default, when a template removes it." + busyUnclear,
	},
	"DeleteUsageLimit": {
		Doc: apiReference + "API_DeleteUsageLimit.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the usage limit exists",
				Break: against(deleteLimit("missing")),
				Code:  notFound,
			},
			{
				Rule:  "a delete leaves the workgroup MODIFYING",
				Check: against(createLimit(), settle, deleteLimit(firstLimit), expectWorkgroup(types.WorkgroupStatusModifying)),
			},
		},
		Unclear: "AWS answered a deleted limit with ResourceNotFoundException in a run against real AWS." + busyUnclear,
	},
	"ListUsageLimits": {
		Doc: apiReference + "API_ListUsageLimits.html",
		Rules: append([]nativetest.Rule{
			{
				Rule: "resourceArn and usageType keep only the usage limits of that workgroup and type",
				Check: against(createLimit(), createLimit(func(in *redshiftserverless.CreateUsageLimitInput) {
					in.UsageType = types.UsageLimitUsageTypeCrossRegionDatasharing
				}), createLimit(func(in *redshiftserverless.CreateUsageLimitInput) { in.ResourceArn = aws.String(workgroupArn("reports")) }),
					func(ctx context.Context, f *fakeAPI) error {
						out, err := f.ListUsageLimits(ctx, &redshiftserverless.ListUsageLimitsInput{
							ResourceArn: aws.String(workgroupArn("analytics")),
							UsageType:   types.UsageLimitUsageTypeServerlessCompute,
						})
						if err != nil {
							return err
						}
						if len(out.UsageLimits) != 1 || aws.ToString(out.UsageLimits[0].UsageLimitId) != firstLimit {
							return fmt.Errorf("the list has %d limits", len(out.UsageLimits))
						}
						return nil
					}),
			},
		}, pagingRules(putLimits, func(ctx context.Context, f *fakeAPI, token *string, maxResults *int32) (int, *string, error) {
			out, err := f.ListUsageLimits(ctx, &redshiftserverless.ListUsageLimitsInput{NextToken: token, MaxResults: maxResults})
			if err != nil {
				return 0, nil, err
			}
			return len(out.UsageLimits), out.NextToken, nil
		})...),
		Unclear: listUnclear + " It doesn't say what AWS returns for a resourceArn of no workgroup; the fake returns " +
			"ValidationException, as CreateUsageLimit does, and Find takes it as no usage limit.",
	},
	"ListWorkgroups": workgroupsCall,
}

// workgroupsCall is the record of ListWorkgroups, which UsageLimit calls to find a workgroup by its ARN.
var workgroupsCall = nativetest.Call{
	Doc: apiReference + "API_ListWorkgroups.html",
	Rules: append([]nativetest.Rule{
		{
			Rule: "a workgroup a change left MODIFYING is AVAILABLE again later, not on the first read",
			Check: against(createLimit(), func(ctx context.Context, f *fakeAPI) error {
				for i, want := range []types.WorkgroupStatus{types.WorkgroupStatusModifying, types.WorkgroupStatusModifying, types.WorkgroupStatusAvailable} {
					out, err := f.ListWorkgroups(ctx, &redshiftserverless.ListWorkgroupsInput{})
					if err != nil {
						return err
					}
					if got := out.Workgroups[0].Status; got != want {
						return fmt.Errorf("read %d: the workgroup is %s, not %s", i, got, want)
					}
				}
				return nil
			}),
		},
	}, pagingRules(func(store *awsfake.CloudControl, n int) {
		// The store already has the fake's workgroups.
		for i := range n - len(workgroups) {
			name := "wg-" + strconv.Itoa(i)
			store.Put(workgroupType, name, map[string]any{"WorkgroupName": name, "WorkgroupArn": workgroupArn(name)})
		}
	}, func(ctx context.Context, f *fakeAPI, token *string, maxResults *int32) (int, *string, error) {
		out, err := f.ListWorkgroups(ctx, &redshiftserverless.ListWorkgroupsInput{NextToken: token, MaxResults: maxResults})
		if err != nil {
			return 0, nil, err
		}
		return len(out.Workgroups), out.NextToken, nil
	})...),
	Unclear: listUnclear + " The page doesn't say how long a change leaves a workgroup MODIFYING; the fake reports it for " +
		"two reads.",
}

// createEndpoint creates the endpoint reads of the workgroup analytics in subnet-1, with the changes edits make to
// the input.
func createEndpoint(edits ...func(*redshiftserverless.CreateEndpointAccessInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &redshiftserverless.CreateEndpointAccessInput{
			EndpointName:        aws.String("reads"),
			WorkgroupName:       aws.String("analytics"),
			SubnetIds:           []string{"subnet-1"},
			VpcSecurityGroupIds: []string{"sg-1"},
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateEndpointAccess(ctx, in)
		return err
	}
}

// expectEndpoint returns a call that reads the endpoint reads and fails unless its status is want.
func expectEndpoint(want string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		out, err := f.GetEndpointAccess(ctx, &redshiftserverless.GetEndpointAccessInput{EndpointName: aws.String("reads")})
		if err != nil {
			return err
		}
		if got := aws.ToString(out.Endpoint.EndpointStatus); got != want {
			return fmt.Errorf("the endpoint is %s, not %s", got, want)
		}
		return nil
	}
}

func updateEndpoint(groups ...string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.UpdateEndpointAccess(ctx, &redshiftserverless.UpdateEndpointAccessInput{EndpointName: aws.String("reads"), VpcSecurityGroupIds: groups})
		return err
	}
}

func deleteEndpoint(ctx context.Context, f *fakeAPI) error {
	_, err := f.DeleteEndpointAccess(ctx, &redshiftserverless.DeleteEndpointAccessInput{EndpointName: aws.String("reads")})
	return err
}

func named(name string) func(*redshiftserverless.CreateEndpointAccessInput) {
	return func(in *redshiftserverless.CreateEndpointAccessInput) { in.EndpointName = aws.String(name) }
}

// endpointAPI records the Redshift Serverless calls EndpointAccess makes.
var endpointAPI = map[string]nativetest.Call{
	"CreateEndpointAccess": {
		Doc: apiReference + "API_CreateEndpointAccess.html",
		Rules: []nativetest.Rule{
			{Rule: "the name is at most 30 characters", Break: against(createEndpoint(named("a123456789012345678901234567890"))), Code: validation},
			{Rule: "the name starts with a letter", Break: against(createEndpoint(named("1reads"))), Code: validation},
			{Rule: "the name has lowercase letters, digits, and hyphens only", Break: against(createEndpoint(named("Reads"))), Code: validation},
			{Rule: "the name has no two hyphens in a row", Break: against(createEndpoint(named("my--reads"))), Code: validation},
			{Rule: "the name doesn't end with a hyphen", Break: against(createEndpoint(named("reads-"))), Code: validation},
			{Rule: "no other endpoint has the name", Break: against(createEndpoint(), createEndpoint()), Code: validation},
			{
				Rule:  "subnetIds is required",
				Break: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) { in.SubnetIds = nil })),
				Code:  validation,
			},
			{
				Rule:  "ownerAccount is an account ID",
				Break: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) { in.OwnerAccount = aws.String("owner") })),
				Code:  validation,
			},
			{
				Rule:  "the workgroup exists",
				Break: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) { in.WorkgroupName = aws.String("missing") })),
				Code:  notFound,
			},
			{
				Rule:  "the workgroup belongs to ownerAccount, or to the caller's account when the create leaves it out",
				Break: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) { in.WorkgroupName = aws.String(sharedWorkgroup) })),
				Code:  notFound,
			},
			{
				Rule: "the owner of a workgroup of another account granted the caller's account access",
				Doc:  "https://docs.aws.amazon.com/redshift/latest/mgmt/serverless-connecting.html#serverless-cross-vpc",
				Break: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) {
					in.WorkgroupName, in.OwnerAccount = aws.String(ungrantedWorkgroup), aws.String(otherAccount)
				})),
				Code: "AccessDeniedException",
			},
			{
				Rule: "an endpoint can reach a workgroup of another account that granted access",
				Check: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) {
					in.WorkgroupName, in.OwnerAccount = aws.String(sharedWorkgroup), aws.String(otherAccount)
				})),
			},
			{Rule: "the workgroup isn't MODIFYING", Break: against(hold("analytics"), createEndpoint()), Code: conflict},
			{
				Rule: "an endpoint whose create sets no security group gets the VPC's default group",
				Check: against(createEndpoint(func(in *redshiftserverless.CreateEndpointAccessInput) { in.VpcSecurityGroupIds = nil }),
					func(ctx context.Context, f *fakeAPI) error {
						out, err := f.GetEndpointAccess(ctx, &redshiftserverless.GetEndpointAccessInput{EndpointName: aws.String("reads")})
						if err != nil {
							return err
						}
						if got := securityGroups(out.Endpoint); len(got) != 1 || got[0] != defaultGroup {
							return fmt.Errorf("the endpoint has the groups %v", got)
						}
						return nil
					}),
			},
			{Rule: "a new endpoint is CREATING, then ACTIVE", Check: against(createEndpoint(), expectEndpoint("CREATING"), expectEndpoint(endpointActive))},
		},
		Unclear: "The page says a name may have uppercase letters, but AWS rejected one with ValidationException (\"can contain: " +
			"lowercase letters, numbers, and hyphens\") in a run against real AWS, so the schema and the fake take lowercase " +
			"only. In that run AWS also answered a name another endpoint has with ValidationException (\"already exists\"), " +
			"a workgroup it doesn't have with ResourceNotFoundException, and a workgroup that was MODIFYING with " +
			"ConflictException (\"An operation is running on the serverless workgroup\"), gave an endpoint created without " +
			"security groups the VPC's default group, and left the workgroup AVAILABLE while the endpoint was CREATING. The " +
			"page doesn't say which subnets AWS takes; the fake takes any. The guide says the owner of a workgroup in another " +
			"account must grant the caller's account access, but not which error AWS returns without it; the fake returns " +
			"AccessDeniedException, which the page lists. The call takes no " +
			"idempotency token: the handler reads the endpoint by its name before it creates one, and reports \"already " +
			"exists\" as an unknown outcome, because an SDK retry of a create whose response was lost gets it.",
	},
	"GetEndpointAccess": {
		Doc: apiReference + "API_GetEndpointAccess.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the endpoint exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetEndpointAccess(ctx, &redshiftserverless.GetEndpointAccessInput{EndpointName: aws.String("missing")})
					return err
				}),
				Code: notFound,
			},
		},
		Unclear: "The page doesn't list an endpoint's statuses. AWS reported CREATING, ACTIVE, MODIFYING, and DELETING in a run " +
			"against real AWS, and no failed status, so the handler waits for ACTIVE until its timeout.",
	},
	"UpdateEndpointAccess": {
		Doc: apiReference + "API_UpdateEndpointAccess.html",
		Rules: []nativetest.Rule{
			{Rule: "the endpoint is ACTIVE", Break: against(createEndpoint(), updateEndpoint("sg-2")), Code: conflict},
			{Rule: "the update sets at least one security group", Break: against(createEndpoint(), expectEndpoint("CREATING"), updateEndpoint()), Code: validation},
			{Rule: "the update changes the security groups", Break: against(createEndpoint(), expectEndpoint("CREATING"), updateEndpoint("sg-1")), Code: validation},
			{
				Rule:  "a changed endpoint is MODIFYING, then ACTIVE",
				Check: against(createEndpoint(), expectEndpoint("CREATING"), updateEndpoint("sg-2"), expectEndpoint("MODIFYING"), expectEndpoint(endpointActive)),
			},
		},
		Unclear: "The page doesn't state these rules. In a run against real AWS, AWS answered an update of an endpoint that " +
			"wasn't ACTIVE with ConflictException (\"status must be active\"), and one with no security group, or with the " +
			"groups the endpoint already had, with ValidationException, and the endpoint was MODIFYING for minutes.",
	},
	"DeleteEndpointAccess": {
		Doc: apiReference + "API_DeleteEndpointAccess.html",
		Rules: []nativetest.Rule{
			{Rule: "the endpoint exists", Break: against(deleteEndpoint), Code: notFound},
			{Rule: "the endpoint is ACTIVE", Break: against(createEndpoint(), deleteEndpoint), Code: conflict},
			{
				Rule: "a deleted endpoint is DELETING, then gone",
				Check: against(createEndpoint(), expectEndpoint("CREATING"), deleteEndpoint, expectEndpoint(endpointDeleting), func(ctx context.Context, f *fakeAPI) error {
					if expectEndpoint("")(ctx, f) == nil {
						return fmt.Errorf("the endpoint is still there")
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't say an endpoint must be ACTIVE. AWS answered a delete of an endpoint that was DELETING with " +
			"ConflictException (\"status must be active\") in a run against real AWS, so the handler reads the endpoint again " +
			"and takes one that is DELETING as deleted.",
	},
	"ListEndpointAccess": {
		Doc: apiReference + "API_ListEndpointAccess.html",
		Rules: pagingRules(func(store *awsfake.CloudControl, n int) {
			for i := range n {
				name := "reads-" + strconv.Itoa(i)
				store.Put(endpointAccessType, name, map[string]any{"EndpointName": name, "WorkgroupName": "analytics", "SubnetIds": []any{"subnet-1"}})
			}
		}, func(ctx context.Context, f *fakeAPI, token *string, maxResults *int32) (int, *string, error) {
			out, err := f.ListEndpointAccess(ctx, &redshiftserverless.ListEndpointAccessInput{NextToken: token, MaxResults: maxResults})
			if err != nil {
				return 0, nil, err
			}
			return len(out.Endpoints), out.NextToken, nil
		}),
		Unclear: listUnclear,
	},
}

func createDomain(edits ...func(*redshiftserverless.CreateCustomDomainAssociationInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &redshiftserverless.CreateCustomDomainAssociationInput{
			WorkgroupName:              aws.String("analytics"),
			CustomDomainName:           aws.String("db.example.com"),
			CustomDomainCertificateArn: aws.String(testCertificate),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateCustomDomainAssociation(ctx, in)
		return err
	}
}

func updateDomain(certificate string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.UpdateCustomDomainAssociation(ctx, &redshiftserverless.UpdateCustomDomainAssociationInput{
			WorkgroupName: aws.String("analytics"), CustomDomainName: aws.String("db.example.com"), CustomDomainCertificateArn: aws.String(certificate),
		})
		return err
	}
}

func deleteDomain(ctx context.Context, f *fakeAPI) error {
	_, err := f.DeleteCustomDomainAssociation(ctx, &redshiftserverless.DeleteCustomDomainAssociationInput{
		WorkgroupName: aws.String("analytics"), CustomDomainName: aws.String("db.example.com"),
	})
	return err
}

// customDomainAPI records the Redshift Serverless calls CustomDomainAssociation makes.
var customDomainAPI = map[string]nativetest.Call{
	"CreateCustomDomainAssociation": {
		Doc: apiReference + "API_CreateCustomDomainAssociation.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the workgroup exists",
				Break: against(createDomain(func(in *redshiftserverless.CreateCustomDomainAssociationInput) { in.WorkgroupName = aws.String("missing") })),
				Code:  notFound,
			},
			{
				Rule:  "the certificate is one ACM issued, not imported or missing",
				Break: against(createDomain(func(in *redshiftserverless.CreateCustomDomainAssociationInput) { in.CustomDomainCertificateArn = aws.String("arn:aws:acm:us-west-2:" + account + ":certificate/missing") })),
				Code:  validation,
			},
			{
				Rule:  "the domain is a domain name of at most 253 characters, with labels that don't start or end with a hyphen",
				Break: against(createDomain(func(in *redshiftserverless.CreateCustomDomainAssociationInput) { in.CustomDomainName = aws.String("-db.example.com") })),
				Code:  validation,
			},
			{
				Rule:  "a workgroup has one custom domain",
				Doc:   domainGuide,
				Break: against(createDomain(), settle, createDomain(func(in *redshiftserverless.CreateCustomDomainAssociationInput) { in.CustomDomainName = aws.String("other.example.com") })),
				Code:  conflict,
			},
			{Rule: "the workgroup isn't MODIFYING", Break: against(hold("analytics"), createDomain()), Code: conflict},
			{Rule: "a create leaves the workgroup MODIFYING", Check: against(createDomain(), expectWorkgroup(types.WorkgroupStatusModifying))},
		},
		Unclear: "AWS answered a certificate it couldn't find, or one it didn't issue, with ValidationException, and a " +
			"workgroup it doesn't have with ResourceNotFoundException, in a run against real AWS. The guide says a workgroup " +
			"has one custom domain and the call needs acm:DescribeCertificate. The page doesn't say whether the change makes " +
			"the workgroup MODIFYING, whether AWS rejects it while the workgroup is, or what a second domain gets; the fake " +
			"does both and answers ConflictException, and the handler waits for the workgroup after every change. The " +
			"recording account had no certificate Amazon issued, so the replay of a successful create is written from the " +
			"API reference." + busyUnclear,
	},
	"GetCustomDomainAssociation": {
		Doc: apiReference + "API_GetCustomDomainAssociation.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the association exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetCustomDomainAssociation(ctx, &redshiftserverless.GetCustomDomainAssociationInput{WorkgroupName: aws.String("analytics"), CustomDomainName: aws.String("db.example.com")})
					return err
				}),
				Code: notFound,
			},
		},
		Unclear: "AWS answered a domain the workgroup doesn't use with ResourceNotFoundException in a run against real AWS. " +
			"The page doesn't say whether AWS stores the domain in lowercase; the schema takes lowercase only, so one domain " +
			"has one identifier.",
	},
	"UpdateCustomDomainAssociation": {
		Doc: apiReference + "API_UpdateCustomDomainAssociation.html",
		Rules: []nativetest.Rule{
			{Rule: "the association exists", Break: against(updateDomain(testCertificate)), Code: notFound},
			{Rule: "the certificate is one ACM issued", Break: against(createDomain(), settle, updateDomain("arn:aws:acm:us-west-2:" + account + ":certificate/missing")), Code: validation},
			{Rule: "the workgroup isn't MODIFYING", Break: against(createDomain(), updateDomain(certificates[1])), Code: conflict},
			{Rule: "an update leaves the workgroup MODIFYING", Check: against(createDomain(), settle, updateDomain(certificates[1]), expectWorkgroup(types.WorkgroupStatusModifying))},
		},
		Unclear: "AWS answered an update of an association it doesn't have with ResourceNotFoundException in a run against " +
			"real AWS. The page doesn't say whether AWS rejects an update while the workgroup is MODIFYING; the fake does." + busyUnclear,
	},
	"DeleteCustomDomainAssociation": {
		Doc: apiReference + "API_DeleteCustomDomainAssociation.html",
		Rules: []nativetest.Rule{
			{Rule: "the association exists", Break: against(deleteDomain), Code: notFound},
			{Rule: "the workgroup isn't MODIFYING", Break: against(createDomain(), deleteDomain), Code: conflict},
			{Rule: "a delete leaves the workgroup MODIFYING", Check: against(createDomain(), settle, deleteDomain, expectWorkgroup(types.WorkgroupStatusModifying))},
		},
		Unclear: "AWS answered a delete of an association it doesn't have with ResourceNotFoundException in a run against " +
			"real AWS. The page doesn't say whether AWS rejects a delete while the workgroup is MODIFYING; the fake does." + busyUnclear,
	},
	"ListCustomDomainAssociations": {
		Doc: apiReference + "API_ListCustomDomainAssociations.html",
		Rules: pagingRules(func(store *awsfake.CloudControl, n int) {
			for i := range n {
				domain := "db" + strconv.Itoa(i) + ".example.com"
				store.Put(customDomainType, "analytics|"+domain, map[string]any{"WorkgroupName": "analytics", "CustomDomainName": domain, "CustomDomainCertificateArn": testCertificate})
			}
		}, func(ctx context.Context, f *fakeAPI, token *string, maxResults *int32) (int, *string, error) {
			out, err := f.ListCustomDomainAssociations(ctx, &redshiftserverless.ListCustomDomainAssociationsInput{NextToken: token, MaxResults: maxResults})
			if err != nil {
				return 0, nil, err
			}
			return len(out.Associations), out.NextToken, nil
		}),
		Unclear: listUnclear,
	},
	"GetWorkgroup": {
		Doc: apiReference + "API_GetWorkgroup.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the workgroup exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String("missing")})
					return err
				}),
				Code: notFound,
			},
			{
				Rule:  "a workgroup a change left MODIFYING is AVAILABLE again later, not on the first read",
				Check: against(createDomain(), expectWorkgroup(types.WorkgroupStatusModifying), expectWorkgroup(types.WorkgroupStatusModifying), expectWorkgroup(types.WorkgroupStatusAvailable)),
			},
		},
		Unclear: "The page doesn't say how long a change leaves a workgroup MODIFYING; the fake reports it for two reads.",
	},
}
