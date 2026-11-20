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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/wI2L/jsondiff"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const managedInsightRuleType = "AWS::CloudWatch::ManagedInsightRule"

// The states of a Contributor Insights rule.
const (
	ruleEnabled  = "ENABLED"
	ruleDisabled = "DISABLED"
)

// The limits CloudWatch's API reference gives for a managed rule.
const (
	maxResourceARN = 1024
	maxRuleTags    = 50
	maxTagKey      = 128
	maxTagValue    = 256
)

// ruleTimeout is how long UCP waits for AWS to stop listing a deleted rule. AWS documents no delay, so this only
// bounds a wait that should be short.
const ruleTimeout = 10 * time.Minute

var (
	// templateNamePattern is the documented pattern of a managed rule's TemplateName.
	templateNamePattern = regexp.MustCompile(`^[0-9A-Za-z][\-\.\_0-9A-Za-z]{0,126}[0-9A-Za-z]$`)

	// tagPattern matches the characters CloudWatch allows in a tag's key and value: letters, numbers, spaces, and
	// . : + = @ _ / -.
	tagPattern = regexp.MustCompile(`^[\p{L}\p{N}\p{Z}.:+=@_/\-]*$`)
)

// InsightRuleAPI is the part of the CloudWatch client the managed Contributor Insights rule handler uses.
type InsightRuleAPI interface {
	PutManagedInsightRules(ctx context.Context, params *cloudwatch.PutManagedInsightRulesInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutManagedInsightRulesOutput, error)
	ListManagedInsightRules(ctx context.Context, params *cloudwatch.ListManagedInsightRulesInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.ListManagedInsightRulesOutput, error)
	EnableInsightRules(ctx context.Context, params *cloudwatch.EnableInsightRulesInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.EnableInsightRulesOutput, error)
	DisableInsightRules(ctx context.Context, params *cloudwatch.DisableInsightRulesInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DisableInsightRulesOutput, error)
	DeleteInsightRules(ctx context.Context, params *cloudwatch.DeleteInsightRulesInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DeleteInsightRulesOutput, error)
	TagResource(ctx context.Context, params *cloudwatch.TagResourceInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.TagResourceOutput, error)
	UntagResource(ctx context.Context, params *cloudwatch.UntagResourceInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.UntagResourceOutput, error)
	ListTagsForResource(ctx context.Context, params *cloudwatch.ListTagsForResourceInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.ListTagsForResourceOutput, error)
	DescribeInsightRules(ctx context.Context, params *cloudwatch.DescribeInsightRulesInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeInsightRulesOutput, error)
}

var (
	_ native.Handler           = (*ManagedInsightRule)(nil)
	_ native.IdentifierEncoder = (*ManagedInsightRule)(nil)
)

// ManagedInsightRule serves AWS::CloudWatch::ManagedInsightRule: a Contributor Insights rule AWS defines for an AWS
// resource, from one of the templates ListManagedInsightRules lists for it. CloudFormation's InsightRule type can't
// make one. The resource and the template identify the rule, so its identifier, ResourceARN and TemplateName joined
// with "|", is known before the create, and AWS names the rule. PutManagedInsightRules is safe to send again: it
// re-enables a rule that is already there. State changes with EnableInsightRules and DisableInsightRules, and Tags
// with TagResource and UntagResource on the rule's ARN, which needs the current account and partition from STS.
type ManagedInsightRule struct {
	api      InsightRuleAPI
	accounts AccountAPI
}

// NewManagedInsightRule returns the handler for managed Contributor Insights rules. It learns the current account,
// for the rule's ARN, with accounts.
func NewManagedInsightRule(api InsightRuleAPI, accounts AccountAPI) *ManagedInsightRule {
	return &ManagedInsightRule{api: api, accounts: accounts}
}

// TypeName returns AWS::CloudWatch::ManagedInsightRule.
func (h *ManagedInsightRule) TypeName() string {
	return managedInsightRuleType
}

// Options returns how UCP uses the handler. AWS lists managed rules only by resource, so the type can't be listed.
func (h *ManagedInsightRule) Options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverByIdentifier,
		DeleteTimeout: ruleTimeout,
	}
}

// ruleID names a managed rule: the resource it is for, and its template.
type ruleID struct {
	resourceARN, template string
}

func (id ruleID) String() string {
	return id.resourceARN + "|" + id.template
}

// ruleInvalid returns the client failure of a template AWS would reject, before any AWS call.
func ruleInvalid(format string, args ...any) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s: %s", managedInsightRuleType, fmt.Sprintf(format, args...)),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// checkResourceARN returns a client failure for a ResourceARN that isn't an ARN of 1 to 1024 characters.
func checkResourceARN(value string) error {
	if utf8.RuneCountInString(value) > maxResourceARN || !arn.IsARN(value) {
		return ruleInvalid("ResourceARN must be the ARN of an AWS resource, at most %d characters", maxResourceARN)
	}
	return nil
}

// checkTemplateName returns a client failure for a TemplateName outside the documented pattern.
func checkTemplateName(value string) error {
	if !templateNamePattern.MatchString(value) {
		return ruleInvalid("TemplateName must be 2 to 128 letters, digits, '-', '.', and '_', starting and ending with a letter or digit")
	}
	return nil
}

// checkRuleTags returns a client failure for Tags that aren't a list of at most 50 tags with distinct keys of 1 to 128
// characters and values of up to 256, of the characters CloudWatch allows. Read leaves out keys that start with aws:,
// which AWS reserves, and the key radius-operation-id, so a template can't set one.
func checkRuleTags(properties map[string]any) error {
	value, present := properties["Tags"]
	if !present || value == nil {
		return nil
	}
	list, ok := value.([]any)
	if !ok || len(list) > maxRuleTags {
		return ruleInvalid("Tags must be a list of at most %d tags", maxRuleTags)
	}
	keys := map[string]bool{}
	for _, item := range list {
		tag, ok := item.(map[string]any)
		key, okKey := tag["Key"].(string)
		tagValue, okValue := tag["Value"].(string)
		if !ok || !okKey || !okValue || utf8.RuneCountInString(key) < 1 || utf8.RuneCountInString(key) > maxTagKey || utf8.RuneCountInString(tagValue) > maxTagValue {
			return ruleInvalid("each tag needs a Key of 1 to %d characters and a Value of up to %d", maxTagKey, maxTagValue)
		}
		if !tagPattern.MatchString(key) || !tagPattern.MatchString(tagValue) {
			return ruleInvalid("the tag %q can have only letters, numbers, spaces, and . : + = @ _ / -", key)
		}
		if keys[key] {
			return ruleInvalid("Tags has the key %q more than once", key)
		}
		if strings.HasPrefix(key, "aws:") || key == awsapi.OperationTag {
			return ruleInvalid("Tags can't have the key %q, which AWS or Radius reserves", key)
		}
		keys[key] = true
	}
	return nil
}

// parseRule checks the properties and returns the rule they name. ok is false when they don't name all of it.
func parseRule(properties map[string]any) (id ruleID, ok bool, err error) {
	strs := map[string]string{}
	for _, name := range []string{"ResourceARN", "TemplateName", "State"} {
		value, present := properties[name]
		if !present || value == nil {
			continue
		}
		s, isString := value.(string)
		if !isString {
			return ruleID{}, false, ruleInvalid("%s must be a string", name)
		}
		strs[name] = s
	}
	if id.resourceARN = strs["ResourceARN"]; id.resourceARN != "" {
		if err := checkResourceARN(id.resourceARN); err != nil {
			return ruleID{}, false, err
		}
	}
	if id.template = strs["TemplateName"]; id.template != "" {
		if err := checkTemplateName(id.template); err != nil {
			return ruleID{}, false, err
		}
	}
	if state, present := strs["State"]; present && state != ruleEnabled && state != ruleDisabled {
		return ruleID{}, false, ruleInvalid("State must be %s or %s", ruleEnabled, ruleDisabled)
	}
	if err := checkRuleTags(properties); err != nil {
		return ruleID{}, false, err
	}
	return id, id.resourceARN != "" && id.template != "", nil
}

// parseRuleID returns the rule an identifier names. A template name has no "|", so the last one ends the ARN.
func parseRuleID(identifier string) (ruleID, error) {
	i := strings.LastIndex(identifier, "|")
	if i < 0 {
		return ruleID{}, ruleInvalid("%q is not the identifier of a managed Contributor Insights rule: it must be ResourceARN|TemplateName", identifier)
	}
	id := ruleID{resourceARN: identifier[:i], template: identifier[i+1:]}
	if err := checkResourceARN(id.resourceARN); err != nil {
		return ruleID{}, err
	}
	if err := checkTemplateName(id.template); err != nil {
		return ruleID{}, err
	}
	return id, nil
}

// Identifier returns ResourceARN|TemplateName, or "" when the properties don't give both. UCP calls it before any AWS
// call, so a template with a value AWS would reject is rejected here.
func (h *ManagedInsightRule) Identifier(properties map[string]any) (string, error) {
	id, ok, err := parseRule(properties)
	if err != nil || !ok {
		return "", err
	}
	return id.String(), nil
}

// desiredState returns the State the properties ask for: ENABLED when they leave it out, as AWS creates a rule.
func desiredState(properties map[string]any) string {
	if state, _ := properties["State"].(string); state != "" {
		return state
	}
	return ruleEnabled
}

// find returns the state of the rule: its name and whether it is enabled. It returns ErrNotFound when AWS lists the
// template for the resource without a rule, which it does for a template no rule was made from.
func (h *ManagedInsightRule) find(ctx context.Context, region string, id ruleID) (*types.ManagedRuleState, error) {
	input := &cloudwatch.ListManagedInsightRulesInput{ResourceARN: aws.String(id.resourceARN)}
	for {
		page, err := h.api.ListManagedInsightRules(ctx, input, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, notFoundCodes...)
		}
		for _, r := range page.ManagedRules {
			if aws.ToString(r.TemplateName) == id.template && r.RuleState != nil && aws.ToString(r.RuleState.RuleName) != "" {
				return r.RuleState, nil
			}
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return nil, native.ErrNotFound
		}
	}
}

// ruleARN returns the ARN of the rule with the name, in the current account and partition, which STS gives. AWS
// returns no ARN for a rule, and TagResource needs it.
func (h *ManagedInsightRule) ruleARN(ctx context.Context, region string, name string) (string, error) {
	out, err := h.accounts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(o *sts.Options) { o.Region = region })
	if err != nil {
		return "", err
	}
	caller, err := arn.Parse(aws.ToString(out.Arn))
	if err != nil || aws.ToString(out.Account) == "" {
		return "", errors.New("STS returned no account ID or ARN for the AWS credentials")
	}
	return arn.ARN{
		Partition: caller.Partition,
		Service:   "cloudwatch",
		Region:    region,
		AccountID: aws.ToString(out.Account),
		Resource:  "insight-rule/" + name,
	}.String(), nil
}

// tags returns the rule's tags, or ErrNotFound when AWS no longer has the rule.
func (h *ManagedInsightRule) tags(ctx context.Context, region string, ruleARN string) ([]types.Tag, error) {
	out, err := h.api.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{ResourceARN: aws.String(ruleARN)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFoundCodes...)
	}
	return out.Tags, nil
}

// Read returns the rule's properties. AWS may report a state in another case, so Read returns it in uppercase.
func (h *ManagedInsightRule) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseRuleID(identifier)
	if err != nil {
		return nil, err
	}
	state, err := h.find(ctx, region, id)
	if err != nil {
		return nil, err
	}
	name := aws.ToString(state.RuleName)
	ruleARN, err := h.ruleARN(ctx, region, name)
	if err != nil {
		return nil, err
	}
	tags, err := h.tags(ctx, region, ruleARN)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{
		"ResourceARN":  id.resourceARN,
		"TemplateName": id.template,
		"State":        strings.ToUpper(aws.ToString(state.State)),
		"RuleName":     name,
		"Arn":          ruleARN,
	}
	if list := awsapi.TagListOf(tags); list != nil {
		properties["Tags"] = list
	}
	return properties, nil
}

// ruleFailures returns the failure AWS reported in a 200 response, for a call that changes one rule, or nil when it
// reported none. AWS doesn't say which failures leave anything changed, so the outcome is unknown.
func ruleFailures(action string, id ruleID, failures []types.PartialFailure) error {
	if len(failures) == 0 {
		return nil
	}
	f := failures[0]
	return &native.Failure{Message: fmt.Sprintf("AWS failed to %s the managed Contributor Insights rule of the template %q for %q (%s %s): %s",
		action, id.template, id.resourceARN, aws.ToString(f.ExceptionType), aws.ToString(f.FailureCode), aws.ToString(f.FailureDescription))}
}

// Create makes the rule with PutManagedInsightRules, with the template's tags. AWS makes a rule enabled, so for a
// template whose State is DISABLED, Create then disables it, which needs the rule's name from AWS's list. When AWS
// doesn't list the new rule yet, Create fails as partly applied, and the next deploy disables it.
func (h *ManagedInsightRule) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, ok, err := parseRule(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ruleInvalid("set ResourceARN and TemplateName")
	}
	rule := types.ManagedRule{ResourceARN: aws.String(id.resourceARN), TemplateName: aws.String(id.template)}
	if tags := awsapi.Tags(request.Properties); len(tags) > 0 {
		rule.Tags = awsapi.SDKTags[types.Tag](tags)
	}
	out, err := h.api.PutManagedInsightRules(ctx, &cloudwatch.PutManagedInsightRulesInput{ManagedRules: []types.ManagedRule{rule}}, inRegion(request.Region))
	if err != nil {
		return "", awsapi.Rejected(err)
	}
	if err := ruleFailures("create", id, out.Failures); err != nil {
		return id.String(), err
	}
	if desiredState(request.Properties) == ruleDisabled {
		if err := h.disableNew(ctx, request.Region, id); err != nil {
			return id.String(), &native.Failure{
				Message: fmt.Sprintf("AWS made the managed Contributor Insights rule of the template %q for %q enabled, and Radius couldn't disable it: %v. Deploy again to disable it", id.template, id.resourceARN, err),
				Outcome: native.OutcomePartial,
			}
		}
	}
	return id.String(), nil
}

// disableNew disables a rule Create just made.
func (h *ManagedInsightRule) disableNew(ctx context.Context, region string, id ruleID) error {
	state, err := h.find(ctx, region, id)
	if errors.Is(err, native.ErrNotFound) {
		return errors.New("AWS doesn't list the new rule yet")
	}
	if err != nil {
		return err
	}
	return h.disable(ctx, region, id, aws.ToString(state.RuleName))
}

// enable enables the rule with the name.
func (h *ManagedInsightRule) enable(ctx context.Context, region string, id ruleID, name string) error {
	out, err := h.api.EnableInsightRules(ctx, &cloudwatch.EnableInsightRulesInput{RuleNames: []string{name}}, inRegion(region))
	if err != nil {
		return err
	}
	return ruleFailures("enable", id, out.Failures)
}

// disable disables the rule with the name.
func (h *ManagedInsightRule) disable(ctx context.Context, region string, id ruleID, name string) error {
	out, err := h.api.DisableInsightRules(ctx, &cloudwatch.DisableInsightRulesInput{RuleNames: []string{name}}, inRegion(region))
	if err != nil {
		return err
	}
	return ruleFailures("disable", id, out.Failures)
}

// changes reports whether the patch of request changes the top-level property name, or replaces every property.
func changes(request native.UpdateRequest, name string) bool {
	return slices.Contains(awsapi.Changed(request), name) || slices.ContainsFunc(request.Patch, func(op jsondiff.Operation) bool { return op.Path == "" })
}

// Update changes the rule's State and Tags, the properties the patch changes, from what AWS has now: it enables or
// disables the rule, and removes and sets tags. A State the template no longer sets goes back to ENABLED.
func (h *ManagedInsightRule) Update(ctx context.Context, request native.UpdateRequest) error {
	id, err := parseRuleID(request.Identifier)
	if err != nil {
		return err
	}
	if _, _, err := parseRule(request.Desired); err != nil {
		return err
	}
	changeState, changeTags := changes(request, "State"), changes(request, "Tags")
	if !changeState && !changeTags {
		return nil
	}
	state, err := h.find(ctx, request.Region, id)
	if err != nil {
		return beforeRuleChange(err)
	}
	name := aws.ToString(state.RuleName)
	calls := []func() error{}
	if want := desiredState(request.Desired); changeState && !strings.EqualFold(aws.ToString(state.State), want) {
		setState := h.disable
		if want == ruleEnabled {
			setState = h.enable
		}
		calls = append(calls, func() error { return setState(ctx, request.Region, id, name) })
	}
	if changeTags {
		ruleARN, err := h.ruleARN(ctx, request.Region, name)
		if err != nil {
			return beforeRuleChange(err)
		}
		current, err := h.tags(ctx, request.Region, ruleARN)
		if err != nil {
			return beforeRuleChange(err)
		}
		set, remove := awsapi.TagChanges(awsapi.Tags(map[string]any{"Tags": awsapi.TagListOf(current)}), awsapi.Tags(request.Desired))
		// Removing first keeps the rule within AWS's 50 tags while a key is replaced.
		if len(remove) > 0 {
			calls = append(calls, func() error {
				_, err := h.api.UntagResource(ctx, &cloudwatch.UntagResourceInput{ResourceARN: aws.String(ruleARN), TagKeys: remove}, inRegion(request.Region))
				return err
			})
		}
		if len(set) > 0 {
			calls = append(calls, func() error {
				_, err := h.api.TagResource(ctx, &cloudwatch.TagResourceInput{ResourceARN: aws.String(ruleARN), Tags: awsapi.SDKTags[types.Tag](set)}, inRegion(request.Region))
				return err
			})
		}
	}
	return awsapi.Rejected(awsapi.Calls(calls...))
}

// beforeRuleChange returns err, from a call made before any call that changes AWS, as a failure AWS applied none of,
// so UCP drops the pending values instead of counting them as possibly applied. ErrNotFound and a native.Failure stay
// as they are.
func beforeRuleChange(err error) error {
	err = awsapi.Rejected(err)
	var failure *native.Failure
	if err == nil || errors.Is(err, native.ErrNotFound) || errors.As(err, &failure) {
		return err
	}
	return &native.Failure{Message: err.Error(), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
}

// Delete deletes the rule. It returns ErrNotFound when AWS lists no rule for the template. When AWS rejects the list,
// such as after the resource was deleted, see unlisted.
func (h *ManagedInsightRule) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseRuleID(identifier)
	if err != nil {
		return err
	}
	state, err := h.find(ctx, region, id)
	if err != nil {
		return beforeRuleChange(h.unlisted(ctx, region, id, err))
	}
	out, err := h.api.DeleteInsightRules(ctx, &cloudwatch.DeleteInsightRulesInput{RuleNames: []string{aws.ToString(state.RuleName)}}, inRegion(region))
	if err != nil {
		return awsapi.Rejected(err)
	}
	return ruleFailures("delete", id, out.Failures)
}

// unlisted returns what a delete does when ListManagedInsightRules failed with err. For a rejection, such as for a
// resource that was deleted, nothing AWS documents ties a rule to its resource or template, so the handler can't find
// the rule. DescribeInsightRules lists every rule in the Region: when none is a managed rule, this one doesn't exist,
// and unlisted returns ErrNotFound, so UCP removes the record. Otherwise it returns a client failure that names the
// managed rules and says what to do. Any other error stays as it is.
func (h *ManagedInsightRule) unlisted(ctx context.Context, region string, id ruleID, err error) error {
	if errors.Is(err, native.ErrNotFound) || !isRejection(err) {
		return err
	}
	managed := []string{}
	input := &cloudwatch.DescribeInsightRulesInput{}
	for {
		page, describeErr := h.api.DescribeInsightRules(ctx, input, inRegion(region))
		if describeErr != nil {
			return fmt.Errorf("AWS can't list the managed Contributor Insights rules of %q (%w), and failed to list the Region's rules: %w", id.resourceARN, err, describeErr)
		}
		for _, r := range page.InsightRules {
			if isManaged(r) {
				managed = append(managed, aws.ToString(r.Name))
			}
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			break
		}
	}
	if len(managed) == 0 {
		return native.ErrNotFound
	}
	return &native.Failure{
		Message: fmt.Sprintf("AWS can't list the managed Contributor Insights rules of the resource %q, which may have been deleted, so Radius can't tell which rule of the template %q is this one: %v. "+
			"The Region has these managed rules: %s. Delete this resource's rule in CloudWatch with DeleteInsightRules, or create the resource again, then delete again. "+
			"If you already deleted the rule, the record stays until the resource exists again or the Region has no managed rules",
			id.resourceARN, id.template, err, strings.Join(managed, ", ")),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// isRejection reports whether err is AWS's rejection of a call as the caller's fault, such as InvalidParameterValue.
func isRejection(err error) bool {
	var failure *native.Failure
	if errors.As(awsapi.Rejected(err), &failure) {
		return failure.ClientError
	}
	return ucp_aws.IsClientFault(err)
}

// isManaged reports whether DescribeInsightRules describes r as a managed rule: ManagedRule is true, or its schema is
// the one AWS documents for managed rules.
func isManaged(r types.InsightRule) bool {
	var schema struct{ Name string }
	_ = json.Unmarshal([]byte(aws.ToString(r.Schema)), &schema)
	return aws.ToBool(r.ManagedRule) || schema.Name == "ServiceLogRule"
}

// List returns nothing, because AWS lists managed rules only by resource.
func (h *ManagedInsightRule) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads AWS's list of the resource's managed rules. PutManagedInsightRules makes the rule before it returns,
// so a create is done once AWS lists the rule, and fails with ErrNotFound when it doesn't, which is how UCP learns
// that a create it lost never reached AWS. A delete is done once AWS doesn't list the rule. An update's calls take
// effect when they return.
func (h *ManagedInsightRule) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	id, err := parseRuleID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.find(ctx, region, id); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationCreate {
		return native.Progress{Done: true, State: "listed"}, nil
	}
	return native.Progress{State: "listed", Waiting: "not listed"}, nil
}
