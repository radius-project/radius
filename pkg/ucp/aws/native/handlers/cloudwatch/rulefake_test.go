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
	"fmt"
	"hash/fnv"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// ruleRegion is the region of the fake's rules.
	ruleRegion = "us-west-2"

	// rulePageSize is how many templates a ListManagedInsightRules page has. AWS doesn't document its page size, and
	// its default MaxResults is 100.
	rulePageSize = 2

	// ruleSettleReads is how many reads of a resource's managed rules still show a deleted rule. AWS doesn't say how
	// soon its list shows the change, so the first read never sees it.
	ruleSettleReads = 2

	// maxRules is the quota of Contributor Insights rules in a region.
	maxRules = 100

	// The error codes of CloudWatch's API reference.
	invalidValue     = "InvalidParameterValue"
	missingParameter = "MissingParameter"
	invalidNextToken = "InvalidNextToken"
	ruleNotFound     = "ResourceNotFoundException"
)

// The resources the fake has managed rule templates for, and their templates.
var (
	endpointService      = "arn:aws:ec2:us-west-2:123456789012:vpc-endpoint-service/vpce-svc-0a1b2c3d4e5f60718"
	otherEndpointService = "arn:aws:ec2:us-west-2:123456789012:vpc-endpoint-service/vpce-svc-0f1e2d3c4b5a69788"

	newConnections      = "VpcEndpointService-NewConnectionsByEndpointId-v1"
	rejectedConnections = "VpcEndpointService-RejectedConnectionsByEndpointId-v1"
	bytesByEndpoint     = "VpcEndpointService-BytesByEndpointId-v1"

	ruleTemplates = map[string][]string{
		endpointService:      {newConnections, rejectedConnections, bytesByEndpoint},
		otherEndpointService: {newConnections, rejectedConnections, bytesByEndpoint},
	}
)

var (
	// ruleNamePattern is the documented pattern of a Contributor Insights rule's name.
	ruleNamePattern = regexp.MustCompile(`^[\x20-\x7E]+$`)

	// fakeTagPattern matches the characters CloudWatch allows in a tag's key and value: letters, numbers, spaces, and
	// . : + = @ _ / -.
	fakeTagPattern = regexp.MustCompile(`^[\p{L}\p{N}\p{Z}.:+=@_/\-]*$`)

	// ruleARNPattern matches a Contributor Insights rule's ARN, and gives its name.
	ruleARNPattern = regexp.MustCompile(`^arn:aws:cloudwatch:` + ruleRegion + `:` + fakeAccount + `:insight-rule/(.+)$`)
)

// ghost is a deleted rule AWS's list still shows.
type ghost struct {
	state *types.ManagedRuleState
	reads int
}

// fakeRuleAPI is an in-memory CloudWatch API for managed Contributor Insights rules, with the STS call that gives the
// current account. It keeps rules in an awsfake store, keyed by their identifier, in the type's property format, so
// the suite's Cloud Control sees the same rules. It rejects the requests CloudWatch's API reference says AWS rejects
// (see ruleAPI), and where the docs are silent it takes the pessimistic side:
//
//   - A deleted rule stays in ListManagedInsightRules for ruleSettleReads reads of its resource's rules, though
//     TagResource and the rule calls already see the change. With createLag set, a created rule stays out of the list
//     for that many reads. It is off by default, because UCP takes a create AWS doesn't list as one that never reached
//     AWS (see Test_ManagedInsightRule_ListLag).
//   - PutManagedInsightRules reports a rule it can't make, for a resource or template it doesn't have, or past the
//     quota, as a failure in a 200 response, and so do the calls that take rule names for a name it doesn't have.
//   - ListManagedInsightRules rejects a resource it has no templates for with InvalidParameterValue.
type fakeRuleAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu sync.Mutex

	// pending holds how many more reads of its resource's rules a created rule stays out of the list, and ghosts the
	// deleted rules the list still shows. createLag is how many reads a new rule stays out of the list.
	pending   map[string]int
	ghosts    map[string]*ghost
	createLag int

	// lowercase makes ListManagedInsightRules return a rule's state in lowercase.
	lowercase bool

	// calls counts the calls that reached the fake, by method.
	calls map[string]int

	// errs maps a method name to an error the call returns instead of acting, and failures to a failure it reports in
	// its response instead of acting.
	errs     map[string]error
	failures map[string]types.PartialFailure
}

var (
	_ InsightRuleAPI = (*fakeRuleAPI)(nil)
	_ AccountAPI     = (*fakeRuleAPI)(nil)
)

func newFakeRuleAPI(store *awsfake.CloudControl) *fakeRuleAPI {
	return &fakeRuleAPI{
		store:    store,
		pending:  map[string]int{},
		ghosts:   map[string]*ghost{},
		calls:    map[string]int{},
		errs:     map[string]error{},
		failures: map[string]types.PartialFailure{},
	}
}

// begin counts a call and returns the error errs holds for it.
func (f *fakeRuleAPI) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// failed returns the failure failures holds for method, as a response's Failures.
func (f *fakeRuleAPI) failed(method string) []types.PartialFailure {
	if failure, ok := f.failures[method]; ok {
		return []types.PartialFailure{failure}
	}
	return nil
}

func (f *fakeRuleAPI) change(apply func()) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	apply()
	return f.failer.After()
}

func (f *fakeRuleAPI) GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetCallerIdentity"); err != nil {
		return nil, err
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(fakeAccount), Arn: aws.String("arn:aws:iam::" + fakeAccount + ":user/radius")}, nil
}

// fakeRuleName returns the name AWS gives the rule of a template for a resource.
func fakeRuleName(resourceARN string, template string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(resourceARN))
	return fmt.Sprintf("%.119s-%08x", template, h.Sum32())
}

// rule returns the stored rule with the identifier, filling in what the record leaves out, as for a record the suite
// writes, or nil when there is none.
func (f *fakeRuleAPI) rule(identifier string) map[string]any {
	p := f.store.Resource(managedInsightRuleType, identifier)
	if p == nil {
		return nil
	}
	id, err := parseRuleID(identifier)
	if err != nil {
		return nil
	}
	if s, _ := p["RuleName"].(string); s == "" {
		p["RuleName"] = fakeRuleName(id.resourceARN, id.template)
	}
	if s, _ := p["State"].(string); s == "" {
		p["State"] = ruleEnabled
	}
	p["Arn"] = "arn:aws:cloudwatch:" + ruleRegion + ":" + fakeAccount + ":insight-rule/" + p["RuleName"].(string)
	return p
}

// byName returns the identifier of the stored rule with the name, or "".
func (f *fakeRuleAPI) byName(name string) string {
	for _, identifier := range f.store.List(managedInsightRuleType) {
		if p := f.rule(identifier); p != nil && p["RuleName"] == name {
			return identifier
		}
	}
	return ""
}

// checkRuleNames checks the rule names a call takes.
func checkRuleNames(names []string) error {
	if len(names) == 0 {
		return ruleFault(missingParameter, "RuleNames is required")
	}
	for _, name := range names {
		if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 128 || !ruleNamePattern.MatchString(name) {
			return ruleFault(invalidValue, "the rule name %q doesn't match the pattern", name)
		}
	}
	return nil
}

// checkTagList checks tags a call sets: at most maxRuleTags, with a Key of 1 to 128 characters and a Value of up to
// 256, of the allowed characters, and no key with the aws: prefix.
func checkTagList(tags []types.Tag) error {
	if len(tags) > maxRuleTags {
		return ruleFault(invalidValue, "a rule can have at most %d tags", maxRuleTags)
	}
	for _, tag := range tags {
		if tag.Key == nil || tag.Value == nil {
			return ruleFault(missingParameter, "a tag needs a Key and a Value")
		}
		if n := utf8.RuneCountInString(*tag.Key); n < 1 || n > maxTagKey || utf8.RuneCountInString(*tag.Value) > maxTagValue {
			return ruleFault(invalidValue, "a tag's Key must be 1 to 128 characters and its Value at most 256")
		}
		if !fakeTagPattern.MatchString(*tag.Key) || !fakeTagPattern.MatchString(*tag.Value) || strings.HasPrefix(*tag.Key, "aws:") {
			return ruleFault(invalidValue, "the tag %q has a character or a prefix AWS doesn't allow", *tag.Key)
		}
	}
	return nil
}

// userTags counts the tags that count toward a rule's limit: those without the aws: prefix.
func userTags(tags []types.Tag) int {
	n := 0
	for _, tag := range tags {
		if !strings.HasPrefix(aws.ToString(tag.Key), "aws:") {
			n++
		}
	}
	return n
}

// checkARN checks a ResourceARN parameter, which must be 1 to 1024 characters.
func checkARN(value *string) error {
	if value == nil {
		return ruleFault(missingParameter, "ResourceARN is required")
	}
	if n := utf8.RuneCountInString(*value); n < 1 || n > maxResourceARN {
		return ruleFault(invalidValue, "ResourceARN must be 1 to 1024 characters")
	}
	return nil
}

func ruleFault(code string, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

// failure returns a failure in a response for the rule or resource.
func failure(code string, resource string, description string) types.PartialFailure {
	return types.PartialFailure{ExceptionType: aws.String(code), FailureCode: aws.String(code), FailureResource: aws.String(resource), FailureDescription: aws.String(description)}
}

func (f *fakeRuleAPI) PutManagedInsightRules(ctx context.Context, in *cloudwatch.PutManagedInsightRulesInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutManagedInsightRulesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutManagedInsightRules"); err != nil {
		return nil, err
	}
	if len(in.ManagedRules) == 0 {
		return nil, ruleFault(missingParameter, "ManagedRules is required")
	}
	for _, r := range in.ManagedRules {
		if err := checkARN(r.ResourceARN); err != nil {
			return nil, err
		}
		if r.TemplateName == nil {
			return nil, ruleFault(missingParameter, "TemplateName is required")
		}
		if !templateNamePattern.MatchString(*r.TemplateName) {
			return nil, ruleFault(invalidValue, "TemplateName doesn't match the pattern")
		}
		if err := checkTagList(r.Tags); err != nil {
			return nil, err
		}
	}
	out := &cloudwatch.PutManagedInsightRulesOutput{Failures: f.failed("PutManagedInsightRules")}
	if len(out.Failures) > 0 {
		return out, nil
	}
	err := f.change(func() {
		for _, r := range in.ManagedRules {
			resource, template := aws.ToString(r.ResourceARN), aws.ToString(r.TemplateName)
			identifier := resource + "|" + template
			if !slices.Contains(ruleTemplates[resource], template) {
				out.Failures = append(out.Failures, failure(invalidValue, resource, fmt.Sprintf("no managed rule template %s for the resource", template)))
				continue
			}
			if p := f.rule(identifier); p != nil {
				// AWS re-enables a rule that is there, and ignores the tags of the call.
				p["State"] = ruleEnabled
				f.store.Put(managedInsightRuleType, identifier, p)
				continue
			}
			if len(f.store.List(managedInsightRuleType)) >= maxRules {
				out.Failures = append(out.Failures, failure("LimitExceededException", resource, "too many Contributor Insights rules"))
				continue
			}
			p := map[string]any{"ResourceARN": resource, "TemplateName": template, "State": ruleEnabled, "RuleName": fakeRuleName(resource, template)}
			if len(r.Tags) > 0 {
				p["Tags"] = tagList(r.Tags)
			}
			f.store.Put(managedInsightRuleType, identifier, p)
			if f.createLag > 0 {
				f.pending[identifier] = f.createLag
			}
			delete(f.ghosts, identifier)
		}
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// tagList returns tags in the type's property format.
func tagList(tags []types.Tag) []any {
	out := []any{}
	for _, tag := range tags {
		out = append(out, map[string]any{"Key": aws.ToString(tag.Key), "Value": aws.ToString(tag.Value)})
	}
	return out
}

// storedTags returns a rule's tags, in the order they were added.
func storedTags(p map[string]any) []types.Tag {
	out := []types.Tag{}
	list, _ := p["Tags"].([]any)
	for _, item := range list {
		tag, _ := item.(map[string]any)
		key, _ := tag["Key"].(string)
		value, _ := tag["Value"].(string)
		out = append(out, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	return out
}

func (f *fakeRuleAPI) ListManagedInsightRules(ctx context.Context, in *cloudwatch.ListManagedInsightRulesInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListManagedInsightRulesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListManagedInsightRules"); err != nil {
		return nil, err
	}
	if err := checkARN(in.ResourceARN); err != nil {
		return nil, err
	}
	if in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 500) {
		return nil, ruleFault(invalidValue, "MaxResults must be 1 to 500")
	}
	resource := aws.ToString(in.ResourceARN)
	templates := slices.Clone(ruleTemplates[resource])
	for _, identifier := range f.store.List(managedInsightRuleType) {
		if id, err := parseRuleID(identifier); err == nil && id.resourceARN == resource && !slices.Contains(templates, id.template) {
			templates = append(templates, id.template)
		}
	}
	if len(templates) == 0 {
		return nil, ruleFault(invalidValue, "the resource %s has no managed rules", resource)
	}
	start := 0
	if in.NextToken != nil {
		var err error
		if start, err = strconv.Atoi(aws.ToString(in.NextToken)); err != nil || start < 0 || start >= len(templates) {
			return nil, ruleFault(invalidNextToken, "the next token is invalid")
		}
	}
	size := rulePageSize
	if in.MaxResults != nil && int(*in.MaxResults) < size {
		size = int(*in.MaxResults)
	}
	end := min(start+size, len(templates))
	out := &cloudwatch.ListManagedInsightRulesOutput{}
	for _, template := range templates[start:end] {
		d := types.ManagedRuleDescription{TemplateName: aws.String(template)}
		if state := f.listed(resource + "|" + template); state != nil {
			d.RuleState = state
			if strings.EqualFold(aws.ToString(state.State), ruleEnabled) {
				d.ResourceARN = aws.String(resource)
			}
		}
		out.ManagedRules = append(out.ManagedRules, d)
	}
	if end < len(templates) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	if in.NextToken == nil {
		// A read of the first page counts as a read of the resource's rules, once it returned what AWS had before.
		f.settle(resource)
	}
	return out, nil
}

// listed returns the state of the rule with the identifier as AWS's list shows it, or nil when the list shows none.
func (f *fakeRuleAPI) listed(identifier string) *types.ManagedRuleState {
	if g, ok := f.ghosts[identifier]; ok {
		return g.state
	}
	p := f.rule(identifier)
	if p == nil || f.pending[identifier] > 0 {
		return nil
	}
	state := p["State"].(string)
	if f.lowercase {
		state = strings.ToLower(state)
	}
	return &types.ManagedRuleState{RuleName: aws.String(p["RuleName"].(string)), State: aws.String(state)}
}

// settle counts a read of the resource's rules toward the changes that list doesn't show yet.
func (f *fakeRuleAPI) settle(resource string) {
	prefix := resource + "|"
	for identifier, n := range f.pending {
		if strings.HasPrefix(identifier, prefix) {
			if n <= 1 {
				delete(f.pending, identifier)
			} else {
				f.pending[identifier] = n - 1
			}
		}
	}
	for identifier, g := range f.ghosts {
		if strings.HasPrefix(identifier, prefix) {
			if g.reads--; g.reads <= 0 {
				delete(f.ghosts, identifier)
			}
		}
	}
}

// setState sets the state of the named rules, and reports the names it doesn't have as failures.
func (f *fakeRuleAPI) setState(method string, names []string, state string) ([]types.PartialFailure, error) {
	if err := f.begin(method); err != nil {
		return nil, err
	}
	if err := checkRuleNames(names); err != nil {
		return nil, err
	}
	if failures := f.failed(method); failures != nil {
		return failures, nil
	}
	failures := []types.PartialFailure{}
	err := f.change(func() {
		for _, name := range names {
			identifier := f.byName(name)
			if identifier == "" {
				failures = append(failures, failure(ruleNotFound, name, "no such rule"))
				continue
			}
			p := f.rule(identifier)
			p["State"] = state
			f.store.Put(managedInsightRuleType, identifier, p)
		}
	})
	return failures, err
}

func (f *fakeRuleAPI) EnableInsightRules(ctx context.Context, in *cloudwatch.EnableInsightRulesInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.EnableInsightRulesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	failures, err := f.setState("EnableInsightRules", in.RuleNames, ruleEnabled)
	if err != nil {
		return nil, err
	}
	return &cloudwatch.EnableInsightRulesOutput{Failures: failures}, nil
}

func (f *fakeRuleAPI) DisableInsightRules(ctx context.Context, in *cloudwatch.DisableInsightRulesInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DisableInsightRulesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	failures, err := f.setState("DisableInsightRules", in.RuleNames, ruleDisabled)
	if err != nil {
		return nil, err
	}
	return &cloudwatch.DisableInsightRulesOutput{Failures: failures}, nil
}

func (f *fakeRuleAPI) DeleteInsightRules(ctx context.Context, in *cloudwatch.DeleteInsightRulesInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DeleteInsightRulesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteInsightRules"); err != nil {
		return nil, err
	}
	if err := checkRuleNames(in.RuleNames); err != nil {
		return nil, err
	}
	out := &cloudwatch.DeleteInsightRulesOutput{Failures: f.failed("DeleteInsightRules")}
	if len(out.Failures) > 0 {
		return out, nil
	}
	err := f.change(func() {
		for _, name := range in.RuleNames {
			identifier := f.byName(name)
			if identifier == "" {
				out.Failures = append(out.Failures, failure(ruleNotFound, name, "no such rule"))
				continue
			}
			p := f.rule(identifier)
			f.ghosts[identifier] = &ghost{state: &types.ManagedRuleState{RuleName: aws.String(name), State: aws.String(p["State"].(string))}, reads: ruleSettleReads}
			delete(f.pending, identifier)
			f.store.Remove(managedInsightRuleType, identifier)
		}
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ruleByARN returns the identifier and properties of the rule with the ARN, or ResourceNotFoundException.
func (f *fakeRuleAPI) ruleByARN(value *string) (string, map[string]any, error) {
	if err := checkARN(value); err != nil {
		return "", nil, err
	}
	m := ruleARNPattern.FindStringSubmatch(aws.ToString(value))
	if m == nil {
		return "", nil, ruleFault(invalidValue, "%s isn't the ARN of a Contributor Insights rule in this account and region", aws.ToString(value))
	}
	identifier := f.byName(m[1])
	if identifier == "" {
		return "", nil, ruleFault(ruleNotFound, "no rule %s", m[1])
	}
	return identifier, f.rule(identifier), nil
}

func (f *fakeRuleAPI) TagResource(ctx context.Context, in *cloudwatch.TagResourceInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.TagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("TagResource"); err != nil {
		return nil, err
	}
	if in.Tags == nil {
		return nil, ruleFault(missingParameter, "Tags is required")
	}
	if err := checkTagList(in.Tags); err != nil {
		return nil, err
	}
	identifier, p, err := f.ruleByARN(in.ResourceARN)
	if err != nil {
		return nil, err
	}
	tags := storedTags(p)
	for _, tag := range in.Tags {
		// A key the rule has keeps its place, with the new value.
		if i := slices.IndexFunc(tags, func(t types.Tag) bool { return aws.ToString(t.Key) == aws.ToString(tag.Key) }); i >= 0 {
			tags[i] = tag
		} else {
			tags = append(tags, tag)
		}
	}
	if userTags(tags) > maxRuleTags {
		return nil, ruleFault(invalidValue, "a rule can have at most %d tags", maxRuleTags)
	}
	if err := f.change(func() {
		p["Tags"] = tagList(tags)
		f.store.Put(managedInsightRuleType, identifier, p)
	}); err != nil {
		return nil, err
	}
	return &cloudwatch.TagResourceOutput{}, nil
}

func (f *fakeRuleAPI) UntagResource(ctx context.Context, in *cloudwatch.UntagResourceInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.UntagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UntagResource"); err != nil {
		return nil, err
	}
	if in.TagKeys == nil {
		return nil, ruleFault(missingParameter, "TagKeys is required")
	}
	for _, key := range in.TagKeys {
		if n := utf8.RuneCountInString(key); n < 1 || n > maxTagKey {
			return nil, ruleFault(invalidValue, "a tag key must be 1 to 128 characters")
		}
	}
	for _, key := range in.TagKeys {
		if strings.HasPrefix(key, "aws:") {
			return nil, ruleFault(invalidValue, "a tag with the aws: prefix can't be removed")
		}
	}
	identifier, p, err := f.ruleByARN(in.ResourceARN)
	if err != nil {
		return nil, err
	}
	tags := slices.DeleteFunc(storedTags(p), func(t types.Tag) bool { return slices.Contains(in.TagKeys, aws.ToString(t.Key)) })
	if err := f.change(func() {
		p["Tags"] = tagList(tags)
		if len(tags) == 0 {
			delete(p, "Tags")
		}
		f.store.Put(managedInsightRuleType, identifier, p)
	}); err != nil {
		return nil, err
	}
	return &cloudwatch.UntagResourceOutput{}, nil
}

func (f *fakeRuleAPI) ListTagsForResource(ctx context.Context, in *cloudwatch.ListTagsForResourceInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListTagsForResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListTagsForResource"); err != nil {
		return nil, err
	}
	_, p, err := f.ruleByARN(in.ResourceARN)
	if err != nil {
		return nil, err
	}
	return &cloudwatch.ListTagsForResourceOutput{Tags: storedTags(p)}, nil
}
