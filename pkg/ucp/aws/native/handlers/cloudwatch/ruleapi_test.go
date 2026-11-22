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

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	managedRuleDoc = apiReference + "API_ManagedRule.html"
	tagDoc         = apiReference + "API_Tag.html"
	taggingDoc     = "https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch-Tagging.html#tagging-restrictions"
	quotasDoc      = "https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/cloudwatch_limits.html"

	// ruleNamesUnclear is what the calls that take rule names leave unclear.
	ruleNamesUnclear = "What AWS reports for a rule name it doesn't have: the fake reports a failure in the 200 response, " +
		"because the page says the response lists the rules it couldn't process."
)

// ruleCall is a call against a fake CloudWatch API for managed rules, after any calls it needs to set up.
type ruleCall func(ctx context.Context, f *fakeRuleAPI) error

// ruleCalls returns calls as a rule's Break or Check, made in order against one fake backed by store. It stops at the
// first call that fails, and returns its error.
func ruleCalls(calls ...ruleCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeRuleAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// putRule returns a call that makes the managed rule of template for endpointService, with the changes edits make.
func putRule(template string, edits ...func(*types.ManagedRule)) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		r := types.ManagedRule{ResourceARN: aws.String(endpointService), TemplateName: aws.String(template)}
		for _, edit := range edits {
			edit(&r)
		}
		out, err := f.PutManagedInsightRules(ctx, &cloudwatch.PutManagedInsightRulesInput{ManagedRules: []types.ManagedRule{r}})
		if err != nil {
			return err
		}
		if len(out.Failures) > 0 {
			return fmt.Errorf("failure: %s", aws.ToString(out.Failures[0].FailureCode))
		}
		return nil
	}
}

// listRules returns the managed rules ListManagedInsightRules lists for resource, from every page, by template.
func listRules(ctx context.Context, f *fakeRuleAPI, resource string) (map[string]*types.ManagedRuleState, error) {
	out := map[string]*types.ManagedRuleState{}
	in := &cloudwatch.ListManagedInsightRulesInput{ResourceARN: aws.String(resource)}
	for {
		page, err := f.ListManagedInsightRules(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, r := range page.ManagedRules {
			out[aws.ToString(r.TemplateName)] = r.RuleState
		}
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return out, nil
		}
	}
}

// settleRules reads endpointService's rules until the list shows every change.
func settleRules(ctx context.Context, f *fakeRuleAPI) error {
	for range ruleSettleReads {
		if _, err := listRules(ctx, f, endpointService); err != nil {
			return err
		}
	}
	return nil
}

// settleTags reads the tags of the rule of template until ListTagsForResource shows every change.
func settleTags(template string) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		for range f.tagLag {
			if _, err := f.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{ResourceARN: aws.String(ruleARNOf(template))}); err != nil {
				return err
			}
		}
		return nil
	}
}

// ruleState returns a call that fails unless the list shows the rule of template with state, or with no rule when
// state is "".
func ruleState(template string, state string) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		rules, err := listRules(ctx, f, endpointService)
		if err != nil {
			return err
		}
		got := ""
		if r := rules[template]; r != nil {
			got = aws.ToString(r.State)
		}
		if got != state {
			return fmt.Errorf("the list shows the rule of %s as %q, not %q", template, got, state)
		}
		return nil
	}
}

// ruleNameOf returns the name of the rule of template for endpointService.
func ruleNameOf(template string) string {
	return fakeRuleName(endpointService, template)
}

// ruleARNOf returns the ARN of the rule of template for endpointService.
func ruleARNOf(template string) string {
	return "arn:aws:cloudwatch:" + ruleRegion + ":" + fakeAccount + ":insight-rule/" + ruleNameOf(template)
}

func tag(key string, value string) types.Tag {
	return types.Tag{Key: aws.String(key), Value: aws.String(value)}
}

// tagRule returns a call that tags the rule of template.
func tagRule(template string, tags ...types.Tag) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		_, err := f.TagResource(ctx, &cloudwatch.TagResourceInput{ResourceARN: aws.String(ruleARNOf(template)), Tags: tags})
		return err
	}
}

// ruleTags returns a call that fails unless the rule of template has exactly want.
func ruleTags(template string, want map[string]string) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		out, err := f.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{ResourceARN: aws.String(ruleARNOf(template))})
		if err != nil {
			return err
		}
		got := map[string]string{}
		for _, t := range out.Tags {
			got[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("the rule has the tags %v, not %v", got, want)
		}
		return nil
	}
}

// ruleNamesCall returns a call of method, EnableInsightRules, DisableInsightRules, or DeleteInsightRules, with names,
// that fails when AWS reports a failure.
func ruleNamesCall(method string, names ...string) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		var failures []types.PartialFailure
		var err error
		switch method {
		case "EnableInsightRules":
			var out *cloudwatch.EnableInsightRulesOutput
			if out, err = f.EnableInsightRules(ctx, &cloudwatch.EnableInsightRulesInput{RuleNames: names}); out != nil {
				failures = out.Failures
			}
		case "DisableInsightRules":
			var out *cloudwatch.DisableInsightRulesOutput
			if out, err = f.DisableInsightRules(ctx, &cloudwatch.DisableInsightRulesInput{RuleNames: names}); out != nil {
				failures = out.Failures
			}
		default:
			var out *cloudwatch.DeleteInsightRulesOutput
			if out, err = f.DeleteInsightRules(ctx, &cloudwatch.DeleteInsightRulesInput{RuleNames: names}); out != nil {
				failures = out.Failures
			}
		}
		if err != nil {
			return err
		}
		if len(failures) > 0 {
			return fmt.Errorf("failure: %s", aws.ToString(failures[0].FailureCode))
		}
		return nil
	}
}

// expectFailure returns a call that succeeds only when call fails with a failure in the response.
func expectFailure(call ruleCall) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		err := call(ctx, f)
		if err == nil || !strings.HasPrefix(err.Error(), "failure: ") {
			return fmt.Errorf("the call didn't report a failure: %v", err)
		}
		return nil
	}
}

// ruleNamesRules returns the rules of a call that takes rule names.
func ruleNamesRules(method string) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "RuleNames is required",
			Break: ruleCalls(ruleNamesCall(method)),
			Code:  missingParameter,
		},
		{
			Rule:  "each rule name is 1 to 128 characters matching [\\x20-\\x7E]+",
			Break: ruleCalls(ruleNamesCall(method, strings.Repeat("r", 129))),
			Code:  invalidValue,
		},
	}
}

// ruleAPI is the record of the AWS calls the managed Contributor Insights rule handler makes.
var ruleAPI = map[string]nativetest.Call{
	"PutManagedInsightRules": {
		Doc: apiReference + "API_PutManagedInsightRules.html",
		Rules: []nativetest.Rule{
			{
				Rule: "ManagedRules is required",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.PutManagedInsightRules(ctx, &cloudwatch.PutManagedInsightRulesInput{})
					return err
				}),
				Code: missingParameter,
			},
			{
				Rule:  "TemplateName is 1 to 128 characters matching [0-9A-Za-z][\\-\\.\\_0-9A-Za-z]{0,126}[0-9A-Za-z]",
				Doc:   managedRuleDoc,
				Break: ruleCalls(putRule("-starts-with-a-dash")),
				Code:  invalidValue,
			},
			{
				Rule:  "ResourceARN is 1 to 1024 characters",
				Doc:   managedRuleDoc,
				Break: ruleCalls(putRule(newConnections, func(r *types.ManagedRule) { r.ResourceARN = aws.String(strings.Repeat("a", 1025)) })),
				Code:  invalidValue,
			},
			{
				Rule: "a rule has at most 50 tags, each with a Key of 1 to 128 characters and a Value of up to 256",
				Doc:  managedRuleDoc,
				Break: ruleCalls(putRule(newConnections, func(r *types.ManagedRule) {
					r.Tags = []types.Tag{tag(strings.Repeat("k", 129), "v")}
				})),
				Code: invalidValue,
			},
			{
				Rule:  "a tag has only letters, numbers, spaces, and . : + = @ _ / -",
				Doc:   taggingDoc,
				Break: ruleCalls(putRule(newConnections, func(r *types.ManagedRule) { r.Tags = []types.Tag{tag("team", "a#b")} })),
				Code:  invalidValue,
			},
			{
				Rule: "the call makes the rule enabled",
				Check: ruleCalls(putRule(newConnections), settleRules, ruleState(newConnections, ruleEnabled),
					ruleTags(newConnections, map[string]string{})),
			},
			{
				Rule: "a call for a rule that is there, and disabled, enables it again, and its tags are ignored",
				Doc:  managedRuleDoc,
				Check: ruleCalls(putRule(newConnections, func(r *types.ManagedRule) { r.Tags = []types.Tag{tag("team", "a")} }), settleRules,
					ruleNamesCall("DisableInsightRules", ruleNameOf(newConnections)), settleRules, ruleState(newConnections, ruleDisabled),
					putRule(newConnections, func(r *types.ManagedRule) { r.Tags = []types.Tag{tag("team", "b")} }),
					settleRules, ruleState(newConnections, ruleEnabled), ruleTags(newConnections, map[string]string{"team": "a"})),
			},
			{
				Rule:  "a region has at most 100 Contributor Insights rules",
				Doc:   quotasDoc,
				Check: ruleCalls(fillRules, expectFailure(putRule(newConnections))),
			},
		},
		Unclear: "What AWS does for a template the resource doesn't have, for a resource that doesn't exist, or past the quota: " +
			"the fake reports a failure in the 200 response. How soon ListManagedInsightRules shows a new rule: the fake shows " +
			"it at once, and Test_ManagedInsightRule_ListLag covers a lag. The ManagedRule page says tags also need cloudwatch:PutInsightRule and cloudwatch:TagResource, " +
			"so the schema asks for both.",
	},
	"ListManagedInsightRules": {
		Doc: apiReference + "API_ListManagedInsightRules.html",
		Rules: []nativetest.Rule{
			{
				Rule: "ResourceARN is required",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.ListManagedInsightRules(ctx, &cloudwatch.ListManagedInsightRulesInput{})
					return err
				}),
				Code: missingParameter,
			},
			{
				Rule: "MaxResults is 1 to 500",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.ListManagedInsightRules(ctx, &cloudwatch.ListManagedInsightRulesInput{ResourceARN: aws.String(endpointService), MaxResults: aws.Int32(501)})
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "a next token the call didn't return fails with InvalidNextToken",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.ListManagedInsightRules(ctx, &cloudwatch.ListManagedInsightRulesInput{ResourceARN: aws.String(endpointService), NextToken: aws.String("x")})
					return err
				}),
				Code: invalidNextToken,
			},
			{
				Rule: "the list has every template of the resource, across pages, with a rule state only for a template a rule was made from",
				Check: ruleCalls(putRule(rejectedConnections), settleRules, func(ctx context.Context, f *fakeRuleAPI) error {
					rules, err := listRules(ctx, f, endpointService)
					if err != nil {
						return err
					}
					if len(rules) != 3 || rules[rejectedConnections] == nil || rules[newConnections] != nil {
						return errors.New("the list doesn't have every template, with a state only for the rule")
					}
					return nil
				}),
			},
			{
				Rule:  "a deleted rule has no rule state",
				Check: ruleCalls(putRule(newConnections), settleRules, ruleNamesCall("DeleteInsightRules", ruleNameOf(newConnections)), settleRules, ruleState(newConnections, "")),
			},
		},
		Unclear: "What AWS returns for a resource that doesn't exist or has no managed rules, or one deleted after its rule " +
			"was made: the fake fails with InvalidParameterValue, and keeps the rule. In what case State is: the fake can return it in lowercase, and the handler reads it in " +
			"uppercase. How soon the list shows a created or deleted rule: the fake shows a deleted rule for two more reads, and a " +
			"created one at once, or after two reads in Test_ManagedInsightRule_ListLag. The page size.",
	},
	"EnableInsightRules": {
		Doc:   apiReference + "API_EnableInsightRules.html",
		Rules: ruleNamesRules("EnableInsightRules"),
		Unclear: ruleNamesUnclear + " When AWS returns LimitExceededException: the fake never does. How soon the list shows the new " +
			"state: the fake shows the old state for one more list, so the handler waits for a list to show it.",
	},
	"DisableInsightRules": {
		Doc: apiReference + "API_DisableInsightRules.html",
		Rules: append(ruleNamesRules("DisableInsightRules"), nativetest.Rule{
			Rule:  "a disabled rule is listed as DISABLED",
			Check: ruleCalls(putRule(newConnections), settleRules, ruleNamesCall("DisableInsightRules", ruleNameOf(newConnections)), settleRules, ruleState(newConnections, ruleDisabled)),
		}),
		Unclear: ruleNamesUnclear + " How soon the list shows the new state: the fake shows the old state for one more " +
			"list, so the handler waits for a list to show it.",
	},
	"DeleteInsightRules": {
		Doc: apiReference + "API_DeleteInsightRules.html",
		Rules: append(ruleNamesRules("DeleteInsightRules"), nativetest.Rule{
			Rule:  "a deleted rule can't be tagged",
			Check: ruleCalls(putRule(newConnections), ruleNamesCall("DeleteInsightRules", ruleNameOf(newConnections)), expectNotFound(tagRule(newConnections, tag("a", "b")))),
		}),
		Unclear: ruleNamesUnclear + " How soon the list stops showing a deleted rule: the fake shows it for two more reads.",
	},
	"TagResource": {
		Doc: apiReference + "API_TagResource.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "a resource has at most 50 tags",
				Break: ruleCalls(putRule(newConnections), tagRule(newConnections, manyTags(51)...)),
				Code:  invalidValue,
			},
			{
				Rule:  "a tag's Key is 1 to 128 characters and its Value up to 256",
				Doc:   tagDoc,
				Break: ruleCalls(putRule(newConnections), tagRule(newConnections, tag("k", strings.Repeat("v", 257)))),
				Code:  invalidValue,
			},
			{
				Rule:  "a rule that doesn't exist fails with ResourceNotFoundException",
				Break: ruleCalls(tagRule(newConnections, tag("a", "b"))),
				Code:  ruleNotFound,
			},
			{
				Rule:  "a tag has only letters, numbers, spaces, and . : + = @ _ / -",
				Doc:   taggingDoc,
				Break: ruleCalls(putRule(newConnections), tagRule(newConnections, tag("team!", "a"))),
				Code:  invalidValue,
			},
			{
				Rule:  "the aws: prefix is reserved for AWS",
				Doc:   taggingDoc,
				Break: ruleCalls(putRule(newConnections), tagRule(newConnections, tag("aws:team", "a"))),
				Code:  invalidValue,
			},
			{
				Rule: "tags with the aws: prefix don't count toward the 50",
				Doc:  taggingDoc,
				Check: ruleCalls(putRule(newConnections), func(ctx context.Context, f *fakeRuleAPI) error {
					identifier := endpointService + "|" + newConnections
					p := f.store.Resource(managedInsightRuleType, identifier)
					p["Tags"] = []any{map[string]any{"Key": "aws:created-by", "Value": "aws"}}
					f.store.Put(managedInsightRuleType, identifier, p)
					return nil
				}, tagRule(newConnections, manyTags(50)...)),
			},
			{
				Rule:  "a tag with a key the resource has replaces its value, and other tags stay",
				Check: ruleCalls(putRule(newConnections), tagRule(newConnections, tag("a", "1"), tag("b", "2")), tagRule(newConnections, tag("a", "3")), settleTags(newConnections), ruleTags(newConnections, map[string]string{"a": "3", "b": "2"})),
			},
		},
		Unclear: "How soon ListTagsForResource shows a change: the fake shows the old tags for one more call, so the handler " +
			"waits for a read to show it. When AWS returns ConcurrentModificationException or ConflictException: the fake never does, because the " +
			"handler makes one change to a rule at a time.",
	},
	"UntagResource": {
		Doc: apiReference + "API_UntagResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "each tag key is 1 to 128 characters",
				Break: ruleCalls(putRule(newConnections), func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.UntagResource(ctx, &cloudwatch.UntagResourceInput{ResourceARN: aws.String(ruleARNOf(newConnections)), TagKeys: []string{""}})
					return err
				}),
				Code: invalidValue,
			},
			{
				Rule: "a rule that doesn't exist fails with ResourceNotFoundException",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.UntagResource(ctx, &cloudwatch.UntagResourceInput{ResourceARN: aws.String(ruleARNOf(newConnections)), TagKeys: []string{"a"}})
					return err
				}),
				Code: ruleNotFound,
			},
		},
		Unclear: "What removing a key the rule doesn't have does: the fake changes nothing. What removing a key with the aws: " +
			"prefix does: the tagging guide says it can't be done, and the fake fails with InvalidParameterValue.",
	},
	"ListTagsForResource": {
		Doc: apiReference + "API_ListTagsForResource.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a rule that doesn't exist fails with ResourceNotFoundException",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{ResourceARN: aws.String(ruleARNOf(newConnections))})
					return err
				}),
				Code: ruleNotFound,
			},
			{
				Rule: "the ARN of a rule is arn:aws:cloudwatch:Region:account-id:insight-rule/insight-rule-name",
				Check: ruleCalls(putRule(newConnections, func(r *types.ManagedRule) { r.Tags = []types.Tag{tag("a", "1")} }),
					ruleTags(newConnections, map[string]string{"a": "1"})),
			},
		},
	},
	"DescribeInsightRules": {
		Doc: apiReference + "API_DescribeInsightRules.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a next token the call didn't return fails with InvalidNextToken",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.DescribeInsightRules(ctx, &cloudwatch.DescribeInsightRulesInput{NextToken: aws.String("x")})
					return err
				}),
				Code: invalidNextToken,
			},
			{
				Rule: "MaxResults is 1 to 500",
				Break: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					_, err := f.DescribeInsightRules(ctx, &cloudwatch.DescribeInsightRulesInput{MaxResults: aws.Int32(501)})
					return err
				}),
				Code: validationError,
			},
			{
				Rule: "the list has every rule in the Region, across pages, and a managed rule has the schema ServiceLogRule",
				Doc:  apiReference + "API_InsightRule.html",
				Check: ruleCalls(putRule(newConnections), putRule(rejectedConnections), func(ctx context.Context, f *fakeRuleAPI) error {
					f.others["custom"] = false
					managed, all := 0, 0
					in := &cloudwatch.DescribeInsightRulesInput{}
					for {
						page, err := f.DescribeInsightRules(ctx, in)
						if err != nil {
							return err
						}
						for _, r := range page.InsightRules {
							all++
							if aws.ToString(r.Schema) == managedSchema {
								managed++
							}
						}
						if in.NextToken = page.NextToken; in.NextToken == nil {
							break
						}
					}
					if all != 3 || managed != 2 {
						return fmt.Errorf("the list has %d rules, %d managed, not 3 and 2", all, managed)
					}
					return nil
				}),
			},
		},
		Unclear: "Whether ManagedRule is set for a rule PutManagedInsightRules makes, since the page calls it a built-in rule: " +
			"the handler also takes the ServiceLogRule schema as managed. Whether a managed rule's name or Definition names its " +
			"resource: the docs don't say, so the handler never matches a rule by them. The error for MaxResults out of range: " +
			"the fake returns ValidationError from Common Errors.",
	},
	"GetCallerIdentity": {
		Doc: "https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the call needs no permissions and returns the account and the ARN of the caller",
				Check: ruleCalls(func(ctx context.Context, f *fakeRuleAPI) error {
					out, err := f.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
					if err != nil {
						return err
					}
					if aws.ToString(out.Account) != fakeAccount || !strings.HasPrefix(aws.ToString(out.Arn), "arn:aws:") {
						return fmt.Errorf("the call returned %q and %q", aws.ToString(out.Account), aws.ToString(out.Arn))
					}
					return nil
				}),
			},
		},
	},
}

// manyTags returns n distinct tags.
func manyTags(n int) []types.Tag {
	out := []types.Tag{}
	for i := range n {
		out = append(out, tag(fmt.Sprintf("k%d", i), "v"))
	}
	return out
}

// fillRules makes the region's quota of Contributor Insights rules.
func fillRules(ctx context.Context, f *fakeRuleAPI) error {
	for i := range maxRules {
		f.store.Put(managedInsightRuleType, fmt.Sprintf("%s|Template-%d", otherEndpointService, i), map[string]any{})
	}
	return nil
}

// expectNotFound returns a call that succeeds only when call fails with ResourceNotFoundException.
func expectNotFound(call ruleCall) ruleCall {
	return func(ctx context.Context, f *fakeRuleAPI) error {
		err := call(ctx, f)
		if err == nil || !strings.Contains(err.Error(), ruleNotFound) {
			return fmt.Errorf("the call didn't fail with %s: %v", ruleNotFound, err)
		}
		return nil
	}
}
