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
	"os"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func handWritten(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

// ruleTemplate returns a template for the rule of template for endpointService.
func ruleTemplate(template string) map[string]any {
	return map[string]any{"ResourceARN": endpointService, "TemplateName": template}
}

func newRuleHandler() (*ManagedInsightRule, *fakeRuleAPI) {
	api := newFakeRuleAPI(awsfake.NewCloudControl())
	return NewManagedInsightRule(api, api), api
}

func Test_Suite_ManagedInsightRule(t *testing.T) {
	properties := with(ruleTemplate(newConnections), map[string]any{"Tags": tags("env", "dev", "team", "network")})
	tooManyTags := []any{}
	for i := range maxRuleTags + 1 {
		tooManyTags = append(tooManyTags, map[string]any{"Key": fmt.Sprintf("k%d", i), "Value": "v"})
	}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    managedInsightRuleType,
		Schema:      handWritten(t, "aws-cloudwatch-managedinsightrule.json"),
		Metadata:    handWritten(t, "aws-cloudwatch-managedinsightrule.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeRuleAPI(store)
			return NewManagedInsightRule(api, api)
		},
		Properties:       properties,
		Identifier:       endpointService + "|" + newConnections,
		Changed:          with(properties, map[string]any{"State": ruleDisabled, "Tags": tags("team", "platform")}),
		Removable:        "Tags",
		CreateOnlyChange: with(properties, map[string]any{"TemplateName": rejectedConnections}),
		// AWS keeps tags as a set, so tags in another order are the same.
		OtherForm: map[string]any{"Tags": tags("team", "network", "env", "dev")},
		Invalid: []map[string]any{
			with(properties, map[string]any{"TemplateName": "-dash"}),
			with(properties, map[string]any{"ResourceARN": "vpce-svc-0a1b2c3d4e5f60718"}),
			with(properties, map[string]any{"State": "enabled"}),
			with(properties, map[string]any{"Tags": tags("a", "1", "a", "2")}),
			with(properties, map[string]any{"Tags": tags("aws:team", "network")}),
			with(properties, map[string]any{"Tags": tags("team!", "network")}),
			with(properties, map[string]any{"Tags": tags("team", "net#work")}),
			with(properties, map[string]any{"Tags": tags("radius-operation-id", "x")}),
			with(properties, map[string]any{"Tags": tooManyTags}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseRuleID(identifier)
			require.NoError(t, err)
			return ruleTemplate(id.template)
		},
		// Disable the rule, set one tag, and remove another.
		MultiCall: with(properties, map[string]any{"State": ruleDisabled, "Tags": tags("team", "platform")}),
		Calls:     3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &h.(*ManagedInsightRule).api.(*fakeRuleAPI).failer
		}),
		API: ruleAPI,
	})
}

// waitRule calls Progress until op finishes, and fails the test when it doesn't within 10 reads.
func waitRule(t *testing.T, h *ManagedInsightRule, identifier string, op native.Operation) {
	t.Helper()
	for range 10 {
		p, err := h.Progress(t.Context(), region, identifier, op)
		if op == native.OperationDelete && errors.Is(err, native.ErrNotFound) {
			return
		}
		require.NoError(t, err)
		if p.Done {
			require.Nil(t, p.Failure)
			return
		}
	}
	require.FailNow(t, "the operation never finished")
}

func createRule(t *testing.T, h *ManagedInsightRule, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	waitRule(t, h, id, native.OperationCreate)
	return id
}

// ruleUpdate returns the update of the rule from what Read returns to desired.
func ruleUpdate(t *testing.T, h *ManagedInsightRule, identifier string, desired map[string]any) native.UpdateRequest {
	t.Helper()
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	readable := with(current, map[string]any{"RuleName": nil, "Arn": nil})
	patch, err := jsondiff.Compare(readable, desired)
	require.NoError(t, err)
	return native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch}
}

// requireRuleFailure checks that err is a native.Failure with outcome, and a message that has want.
func requireRuleFailure(t *testing.T, err error, outcome native.Outcome, want string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, outcome, failure.Outcome, "%v", err)
	require.Contains(t, failure.Message, want)
}

func Test_ManagedInsightRule_Identifier(t *testing.T) {
	h, api := newRuleHandler()
	id, err := h.Identifier(ruleTemplate(newConnections))
	require.NoError(t, err)
	require.Equal(t, endpointService+"|"+newConnections, id)
	for _, partial := range []map[string]any{{"ResourceARN": endpointService}, {"TemplateName": newConnections}} {
		id, err := h.Identifier(partial)
		require.NoError(t, err)
		require.Empty(t, id)
	}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ResourceARN": endpointService}})
	requireRuleFailure(t, err, native.OutcomeRejected, "set ResourceARN and TemplateName")

	// An ARN with "|" is split at the last one, because a template name can't have one.
	parsed, err := parseRuleID("arn:aws:s3:::a|b|" + newConnections)
	require.NoError(t, err)
	require.Equal(t, ruleID{resourceARN: "arn:aws:s3:::a|b", template: newConnections}, parsed)

	for _, identifier := range []string{newConnections, "x|" + newConnections, endpointService + "|-x", endpointService + "|"} {
		_, err := h.Read(t.Context(), region, identifier)
		requireRuleFailure(t, err, native.OutcomeRejected, "")
		requireRuleFailure(t, h.Delete(t.Context(), region, identifier), native.OutcomeRejected, "")
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		requireRuleFailure(t, err, native.OutcomeRejected, "")
	}
	for _, properties := range []map[string]any{
		with(ruleTemplate(newConnections), map[string]any{"State": 1}),
		with(ruleTemplate(newConnections), map[string]any{"Tags": "team"}),
		with(ruleTemplate(newConnections), map[string]any{"Tags": []any{map[string]any{"Key": "", "Value": "v"}}}),
		with(ruleTemplate(newConnections), map[string]any{"ResourceARN": "arn:aws:ec2:us-west-2:1:" + string(make([]byte, maxResourceARN))}),
	} {
		_, err := h.Identifier(properties)
		requireRuleFailure(t, err, native.OutcomeRejected, "")
	}
	require.Zero(t, api.calls["ListManagedInsightRules"], "nothing invalid reaches AWS")
}

// Test_ManagedInsightRule_FirstLook checks the rule a create makes, and that a deleted rule AWS still lists keeps the
// delete waiting.
func Test_ManagedInsightRule_FirstLook(t *testing.T) {
	h, _ := newRuleHandler()
	id := createRule(t, h, ruleTemplate(newConnections))
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"ResourceARN": endpointService, "TemplateName": newConnections, "State": ruleEnabled,
		"RuleName": ruleNameOf(newConnections), "Arn": ruleARNOf(newConnections),
	}, got)

	require.NoError(t, h.Delete(t.Context(), region, id))
	p, err := h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, p.Done, "AWS still lists the deleted rule")
	waitRule(t, h, id, native.OperationDelete)
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound)
}

// Test_ManagedInsightRule_ListLag checks what happens if AWS lists a new rule only after a while, which its docs don't
// rule out. UCP takes a create AWS doesn't list as one that never reached AWS, so the create fails asking for another
// deploy. That deploy finds the rule, or sends the create again, which AWS takes as the same rule, so AWS never has two.
func Test_ManagedInsightRule_ListLag(t *testing.T) {
	h, api := newRuleHandler()
	api.createLag = 2
	properties := with(ruleTemplate(newConnections), map[string]any{"Tags": tags("team", "a")})
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)

	// The next deploy reads AWS, which may still not list the rule, and sends the create again.
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound)
	again, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, id, again)
	waitRule(t, h, id, native.OperationCreate)
	require.Len(t, api.store.List(managedInsightRuleType), 1)
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, tags("team", "a"), got["Tags"])
}

// Test_ManagedInsightRule_CreateAgain checks that a create sent again, as UCP does after it lost the first, makes no
// second rule.
func Test_ManagedInsightRule_CreateAgain(t *testing.T) {
	h, api := newRuleHandler()
	properties := with(ruleTemplate(newConnections), map[string]any{"Tags": tags("team", "a")})
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	again, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, id, again)
	waitRule(t, h, id, native.OperationCreate)
	require.Len(t, api.store.List(managedInsightRuleType), 1)
}

// Test_ManagedInsightRule_ResponseFailures checks that a failure AWS reports in a 200 response of each call that
// changes a rule fails the operation, with AWS's message, instead of being taken as done.
func Test_ManagedInsightRule_ResponseFailures(t *testing.T) {
	h, api := newRuleHandler()
	api.failures["PutManagedInsightRules"] = failure("LimitExceededException", endpointService, "too many rules")
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: ruleTemplate(newConnections)})
	require.Equal(t, endpointService+"|"+newConnections, id)
	requireRuleFailure(t, err, native.OutcomeUnknown, "too many rules")
	require.Empty(t, api.store.List(managedInsightRuleType))

	// A resource or a template AWS doesn't have.
	delete(api.failures, "PutManagedInsightRules")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(ruleTemplate("Other-Template"), nil)})
	requireRuleFailure(t, err, native.OutcomeUnknown, "no managed rule template")

	id = createRule(t, h, ruleTemplate(newConnections))
	for _, method := range []string{"DisableInsightRules", "DeleteInsightRules"} {
		api.failures[method] = failure("InternalServiceError", ruleNameOf(newConnections), method+" failed")
	}
	err = h.Update(t.Context(), ruleUpdate(t, h, id, with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled})))
	requireRuleFailure(t, err, native.OutcomeUnknown, "DisableInsightRules failed")
	requireRuleFailure(t, h.Delete(t.Context(), region, id), native.OutcomeUnknown, "DeleteInsightRules failed")
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, ruleEnabled, got["State"])
}

// Test_ManagedInsightRule_CreateDisabled checks a template that asks for a disabled rule. AWS makes a rule enabled, so
// Create disables it once AWS lists it, and otherwise fails as partly applied, and the next deploy disables it.
func Test_ManagedInsightRule_CreateDisabled(t *testing.T) {
	h, api := newRuleHandler()
	api.createLag = 1
	properties := with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled})
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	requireRuleFailure(t, err, native.OutcomePartial, "Deploy again to disable it")
	require.Equal(t, endpointService+"|"+newConnections, id)
	waitRule(t, h, id, native.OperationCreate)
	require.NoError(t, h.Update(t.Context(), ruleUpdate(t, h, id, properties)))
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, ruleDisabled, got["State"])

	// When AWS lists the rule at once, the create disables it.
	h, api = newRuleHandler()
	properties = with(ruleTemplate(rejectedConnections), map[string]any{"State": ruleDisabled})
	id, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	got, err = h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, ruleDisabled, got["State"])
	require.Equal(t, 1, api.calls["DisableInsightRules"])
}

// Test_ManagedInsightRule_StateCase checks that a state AWS returns in lowercase reads as the template's, so a deploy
// of the same template changes nothing.
func Test_ManagedInsightRule_StateCase(t *testing.T) {
	h, api := newRuleHandler()
	api.lowercase = true
	properties := with(ruleTemplate(newConnections), map[string]any{"State": ruleEnabled})
	id := createRule(t, h, properties)
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, ruleEnabled, got["State"])

	// A patch that names State, such as after an update UCP can't confirm, sends nothing when AWS has the state.
	request := ruleUpdate(t, h, id, properties)
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/State", Value: ruleEnabled}}
	require.NoError(t, h.Update(t.Context(), request))
	require.Zero(t, api.calls["EnableInsightRules"])
}

// Test_ManagedInsightRule_Update checks that an update changes only what its patch names, from what AWS has now, and
// that a State the template no longer sets goes back to ENABLED.
func Test_ManagedInsightRule_Update(t *testing.T) {
	h, api := newRuleHandler()
	properties := with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled, "Tags": tags("a", "1", "b", "2")})
	id := createRule(t, h, with(properties, map[string]any{"State": nil}))
	require.NoError(t, h.Update(t.Context(), ruleUpdate(t, h, id, properties)))

	// Tags change outside Radius. A patch that names only State leaves them.
	_, err := api.TagResource(t.Context(), tagInput(ruleARNOf(newConnections), tag("c", "3")))
	require.NoError(t, err)
	request := ruleUpdate(t, h, id, with(properties, map[string]any{"State": nil}))
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/State"}}
	require.NoError(t, h.Update(t.Context(), request))
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, ruleEnabled, got["State"])
	require.ElementsMatch(t, tags("a", "1", "b", "2", "c", "3"), got["Tags"])

	// A patch that names Tags sets the template's and removes the rest, read from AWS, not from the request.
	request = ruleUpdate(t, h, id, with(properties, map[string]any{"State": nil, "Tags": tags("a", "9")}))
	request.Current = with(request.Current, map[string]any{"Tags": nil})
	require.NoError(t, h.Update(t.Context(), request))
	got, err = h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, tags("a", "9"), got["Tags"])

	// A patch that replaces every property changes both.
	request = ruleUpdate(t, h, id, properties)
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "", Value: properties}}
	require.NoError(t, h.Update(t.Context(), request))
	got, err = h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, ruleDisabled, got["State"])
	require.Equal(t, tags("a", "1", "b", "2"), got["Tags"])

	// An update that changes nothing reads nothing.
	calls := api.calls["ListManagedInsightRules"]
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: properties}))
	require.Equal(t, calls, api.calls["ListManagedInsightRules"])
	p, err := h.Progress(t.Context(), region, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
}

// Test_ManagedInsightRule_AWSTags checks that tags AWS adds are never read, so a deploy never removes them.
func Test_ManagedInsightRule_AWSTags(t *testing.T) {
	h, api := newRuleHandler()
	id := createRule(t, h, ruleTemplate(newConnections))
	// Only AWS can add a tag with the aws: prefix, so it goes straight into the store.
	stored := api.store.Resource(managedInsightRuleType, id)
	stored["Tags"] = tags("aws:cloudformation:stack-name", "s")
	api.store.Put(managedInsightRuleType, id, stored)
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.NotContains(t, got, "Tags")

	require.NoError(t, h.Update(t.Context(), ruleUpdate(t, h, id, with(ruleTemplate(newConnections), map[string]any{"Tags": tags("a", "1")}))))
	require.Zero(t, api.calls["UntagResource"])
}

// Test_ManagedInsightRule_FailsBeforeChange checks that a read that fails before an update or delete sends anything
// is reported as rejected, so UCP drops the update's values instead of counting them as possibly applied.
func Test_ManagedInsightRule_FailsBeforeChange(t *testing.T) {
	h, api := newRuleHandler()
	id := createRule(t, h, ruleTemplate(newConnections))
	disable := ruleUpdate(t, h, id, with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled, "Tags": tags("a", "1")}))
	for _, method := range []string{"ListManagedInsightRules", "GetCallerIdentity", "ListTagsForResource"} {
		api.errs[method] = errors.New(method + " down")
		requireRuleFailure(t, h.Update(t.Context(), disable), native.OutcomeRejected, method+" down")
		delete(api.errs, method)
	}
	api.errs["ListManagedInsightRules"] = errors.New("list down")
	requireRuleFailure(t, h.Delete(t.Context(), region, id), native.OutcomeRejected, "list down")
	require.Zero(t, api.calls["DisableInsightRules"]+api.calls["TagResource"]+api.calls["DeleteInsightRules"])
}

// Test_ManagedInsightRule_CallErrors checks how the errors of the calls that change AWS are reported: the first call
// of an update AWS rejects changed nothing, and a later one fails partly applied.
func Test_ManagedInsightRule_CallErrors(t *testing.T) {
	h, api := newRuleHandler()
	id := createRule(t, h, ruleTemplate(newConnections))
	rejected := &smithy.GenericAPIError{Code: invalidValue, Message: "bad", Fault: smithy.FaultClient}

	api.errs["DisableInsightRules"] = rejected
	err := h.Update(t.Context(), ruleUpdate(t, h, id, with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled, "Tags": tags("a", "1")})))
	require.ErrorIs(t, err, rejected)
	require.Zero(t, api.calls["TagResource"])

	delete(api.errs, "DisableInsightRules")
	api.errs["TagResource"] = rejected
	err = h.Update(t.Context(), ruleUpdate(t, h, id, with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled, "Tags": tags("a", "1")})))
	requireRuleFailure(t, err, native.OutcomePartial, "bad")

	api.errs["PutManagedInsightRules"] = rejected
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: ruleTemplate(rejectedConnections)})
	require.ErrorIs(t, err, rejected)

	api.errs["DeleteInsightRules"] = errors.New("delete down")
	require.ErrorContains(t, h.Delete(t.Context(), region, id), "delete down")

	// A rule deleted between the list and the tags reads as not found.
	api.errs["ListTagsForResource"] = &smithy.GenericAPIError{Code: ruleNotFound, Fault: smithy.FaultClient}
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound)

	// STS without an account or ARN.
	delete(api.errs, "ListTagsForResource")
	h = NewManagedInsightRule(api, noIdentity{})
	_, err = h.Read(t.Context(), region, id)
	require.ErrorContains(t, err, "no account ID")
}

// Test_ManagedInsightRule_Pages checks that a rule on a later page of the resource's templates is found, and that List
// returns nothing, because AWS lists managed rules only by resource.
func Test_ManagedInsightRule_Pages(t *testing.T) {
	h, _ := newRuleHandler()
	id := createRule(t, h, ruleTemplate(bytesByEndpoint))
	_, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	_, err = h.Read(t.Context(), region, endpointService+"|"+newConnections)
	require.ErrorIs(t, err, native.ErrNotFound)
	list, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, list)
}

func tagInput(arn string, tags ...types.Tag) *cloudwatch.TagResourceInput {
	return &cloudwatch.TagResourceInput{ResourceARN: aws.String(arn), Tags: tags}
}

// noIdentity is an STS client that returns no account or ARN.
type noIdentity struct{}

func (noIdentity) GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{}, nil
}

// Test_ManagedInsightRule_InvalidInputs checks that a create or update with a value AWS would reject, or of an
// identifier that isn't a rule's, sends nothing, and that the errors of the calls that set State are returned.
func Test_ManagedInsightRule_InvalidInputs(t *testing.T) {
	h, api := newRuleHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(ruleTemplate(newConnections), map[string]any{"State": "PAUSED"})})
	requireRuleFailure(t, err, native.OutcomeRejected, "State must be")
	requireRuleFailure(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "x"}), native.OutcomeRejected, "is not the identifier")
	id := createRule(t, h, ruleTemplate(newConnections))
	requireRuleFailure(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: map[string]any{"Tags": "x"}}), native.OutcomeRejected, "Tags must be")
	require.Equal(t, 1, api.calls["PutManagedInsightRules"])
	require.Equal(t, 1, api.calls["ListManagedInsightRules"], "only the create's wait read AWS")

	// The calls that set State.
	disabled := with(ruleTemplate(newConnections), map[string]any{"State": ruleDisabled})
	require.NoError(t, h.Update(t.Context(), ruleUpdate(t, h, id, disabled)))
	api.errs["EnableInsightRules"] = errors.New("enable down")
	require.ErrorContains(t, h.Update(t.Context(), ruleUpdate(t, h, id, ruleTemplate(newConnections))), "enable down")

	// A create whose list fails after the rule was made fails partly applied.
	api.errs["ListManagedInsightRules"] = errors.New("list down")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(disabled, map[string]any{"TemplateName": rejectedConnections})})
	requireRuleFailure(t, err, native.OutcomePartial, "list down")
}

// Test_ManagedInsightRule_FullTags checks that an update of a rule with 50 tags that replaces a key removes the old key
// before it adds the new one, so the rule never has more than AWS allows. Tags AWS adds don't count.
func Test_ManagedInsightRule_FullTags(t *testing.T) {
	h, api := newRuleHandler()
	full := []any{}
	for i := range maxRuleTags {
		full = append(full, map[string]any{"Key": fmt.Sprintf("k%02d", i), "Value": "v"})
	}
	properties := with(ruleTemplate(newConnections), map[string]any{"Tags": full})
	id := createRule(t, h, properties)
	stored := api.store.Resource(managedInsightRuleType, id)
	stored["Tags"] = append(stored["Tags"].([]any), map[string]any{"Key": "aws:created-by", "Value": "aws"})
	api.store.Put(managedInsightRuleType, id, stored)

	replaced := append(slices.Clone(full[1:]), map[string]any{"Key": "new", "Value": "v"})
	require.NoError(t, h.Update(t.Context(), ruleUpdate(t, h, id, with(properties, map[string]any{"Tags": replaced}))))
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.ElementsMatch(t, replaced, got["Tags"])
}
