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

package ses

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	identityPolicyType = "AWS::SES::EmailIdentityPolicy"

	// alreadyExistsCode is the error SES returns for a create of a policy the identity already has.
	alreadyExistsCode = "AlreadyExistsException"

	// defaultPolicyVersion is the Version SES gives a policy that has none.
	defaultPolicyVersion = "2008-10-17"
)

// policyName matches a name SES takes for a policy.
var policyName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// rootARN matches the ARN of an account's root user, which SES stores in place of an account ID in a principal.
var rootARN = regexp.MustCompile(`^arn:[a-z0-9-]+:iam::([0-9]{12}):root$`)

var (
	_ native.Handler           = (*EmailIdentityPolicy)(nil)
	_ native.IdentifierEncoder = (*EmailIdentityPolicy)(nil)
)

// EmailIdentityPolicy serves AWS::SES::EmailIdentityPolicy, a sending authorization policy of an SES email identity.
// Its identifier is the identity and the policy's name, joined with "|". A delete deletes the policy.
type EmailIdentityPolicy struct {
	api  V2API
	wait pause
}

// NewEmailIdentityPolicy returns the handler for an email identity's sending authorization policies.
func NewEmailIdentityPolicy(api V2API) *EmailIdentityPolicy {
	return &EmailIdentityPolicy{api: api, wait: sleep}
}

// TypeName returns AWS::SES::EmailIdentityPolicy.
func (h *EmailIdentityPolicy) TypeName() string {
	return identityPolicyType
}

// Options returns how UCP uses the handler. SES stores a policy in its own form, so a policy in another form that
// means the same counts as unchanged.
func (h *EmailIdentityPolicy) Options() native.Options {
	o := options()
	o.Equivalent = func(path string, template any, aws any) bool {
		t, tok := template.(string)
		a, aok := aws.(string)
		return path == "Policy" && tok && aok && samePolicy(t, a)
	}
	return o
}

// Identifier returns EmailIdentity and PolicyName, joined with "|". An identity's ARN, or a name SES doesn't take, is
// rejected before any call.
func (h *EmailIdentityPolicy) Identifier(properties map[string]any) (string, error) {
	if _, err := identityOf(properties, "EmailIdentity"); err != nil {
		return "", err
	}
	if name := awsapi.String(properties, "PolicyName"); name != "" && !policyName.MatchString(name) {
		return "", rejected("PolicyName must have 1 to 64 letters, digits, dashes, and underscores")
	}
	return joined(properties, "EmailIdentity", "PolicyName"), nil
}

// Read returns the policy, or ErrNotFound when the identity has no policy with the name or SES has no such identity.
func (h *EmailIdentityPolicy) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	identity, name, err := pair(identifier)
	if err != nil {
		return nil, err
	}
	out, err := h.api.GetEmailIdentityPolicies(ctx, &sesv2.GetEmailIdentityPoliciesInput{EmailIdentity: aws.String(identity)}, inRegionV2(region))
	if err != nil {
		return nil, readError(err)
	}
	policy, ok := out.Policies[name]
	if !ok {
		return nil, native.ErrNotFound
	}
	return map[string]any{"EmailIdentity": identity, "PolicyName": name, "Policy": policy}, nil
}

// shows returns whether a read shows policy, in any form that means the same.
func shows(policy string) func(map[string]any) bool {
	return func(got map[string]any) bool {
		return samePolicy(policy, awsapi.String(got, "Policy"))
	}
}

// policyOf returns the Policy properties set, or a client failure when they set none.
func policyOf(properties map[string]any) (string, error) {
	policy := awsapi.String(properties, "Policy")
	if policy == "" {
		return "", rejected("set Policy")
	}
	return policy, nil
}

// Create creates the policy, waits until a read shows it, and returns its identifier. A create SES answers with
// AlreadyExistsException, or that no read shows, fails with an unknown outcome and keeps the identifier.
func (h *EmailIdentityPolicy) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, _ := h.Identifier(request.Properties)
	identity, name, err := pair(identifier)
	if err != nil {
		return "", rejected("set EmailIdentity and PolicyName")
	}
	policy, err := policyOf(request.Properties)
	if err != nil {
		return "", err
	}
	_, err = h.api.CreateEmailIdentityPolicy(ctx, &sesv2.CreateEmailIdentityPolicyInput{
		EmailIdentity: aws.String(identity),
		PolicyName:    aws.String(name),
		Policy:        aws.String(policy),
	}, inRegionV2(request.Region))
	if awsapi.HasCode(err, alreadyExistsCode) {
		// UCP creates a policy only after a read found none, so SES has it because this create, or an SDK retry of
		// it whose first response was lost, made it. Report the outcome as unknown, which keeps the identifier, so
		// the next deploy reads the policy and updates it.
		return identifier, &native.Failure{Message: fmt.Sprintf("SES already has the policy %q. Deploy again.", identifier), Outcome: native.OutcomeUnknown}
	}
	if err != nil {
		// The identifier is known, and a create whose answer UCP didn't get, such as a timeout, may have made the
		// policy, so it is kept with every error that isn't a rejection.
		return identifier, awsapi.Rejected(err)
	}
	if err := confirm(ctx, h.wait, h.Read, request.Region, identifier, "the policy", shows(policy)); err != nil {
		return identifier, err
	}
	return identifier, nil
}

// Update replaces the policy with the template's, and waits until a read shows it.
func (h *EmailIdentityPolicy) Update(ctx context.Context, request native.UpdateRequest) error {
	identity, name, err := pair(request.Identifier)
	if err != nil {
		return err
	}
	policy, err := policyOf(request.Desired)
	if err != nil {
		return err
	}
	_, err = h.api.UpdateEmailIdentityPolicy(ctx, &sesv2.UpdateEmailIdentityPolicyInput{
		EmailIdentity: aws.String(identity),
		PolicyName:    aws.String(name),
		Policy:        aws.String(policy),
	}, inRegionV2(request.Region))
	if err != nil {
		return awsapi.Rejected(err)
	}
	return confirm(ctx, h.wait, h.Read, request.Region, request.Identifier, "the policy", shows(policy))
}

// Delete deletes the policy. It returns ErrNotFound when SES has no such policy or identity.
func (h *EmailIdentityPolicy) Delete(ctx context.Context, region string, identifier string) error {
	identity, name, err := pair(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteEmailIdentityPolicy(ctx, &sesv2.DeleteEmailIdentityPolicyInput{EmailIdentity: aws.String(identity), PolicyName: aws.String(name)}, inRegionV2(region))
	return notFoundOrRejected(err)
}

// List fails, because the policies can't be listed without reading every identity.
func (h *EmailIdentityPolicy) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, cantList(identityPolicyType)
}

// Progress reports a create as done once a read finds the policy, an update as done, because the update waited until
// a read showed it, and a delete as done, with ErrNotFound, once a read no longer finds the policy.
func (h *EmailIdentityPolicy) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	switch op {
	case native.OperationCreate:
		return createProgress(ctx, h.Read, region, identifier)
	case native.OperationDelete:
		return deleteProgress(ctx, h.Read, region, identifier)
	}
	return native.Progress{Done: true}, nil
}

// samePolicy reports whether two policies, as JSON, mean the same in the forms SES stores them: with any whitespace
// and key order, a list in any order and without duplicates, a list of one value as the value, an account ID in a
// principal as the account's root ARN, and Version 2008-10-17 when the policy has none. It reports false when either
// isn't JSON.
func samePolicy(a string, b string) bool {
	ca, ok := canonicalPolicy(a)
	if !ok {
		return false
	}
	cb, ok := canonicalPolicy(b)
	return ok && reflect.DeepEqual(ca, cb)
}

// canonicalPolicy returns policy in one form for each meaning, as samePolicy describes, and whether it is a JSON
// object.
func canonicalPolicy(policy string) (any, bool) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(policy), &doc); err != nil || doc == nil {
		return nil, false
	}
	if _, ok := doc["Version"]; !ok {
		doc["Version"] = defaultPolicyVersion
	}
	return canonical(doc, false), true
}

// canonical returns v in one form for each meaning. principal is true inside the AWS principals of a statement,
// where an account's root ARN means its account ID.
func canonical(v any, principal bool) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, value := range v {
			switch key {
			case "Principal", "NotPrincipal":
				if principals, ok := value.(map[string]any); ok {
					value = withAWSPrincipals(principals)
				}
			}
			out[key] = canonical(value, false)
		}
		return out
	case []any:
		type item struct {
			key   string
			value any
		}
		items := []item{}
		for _, value := range v {
			c := canonical(value, principal)
			b, _ := json.Marshal(c)
			if !slices.ContainsFunc(items, func(i item) bool { return i.key == string(b) }) {
				items = append(items, item{key: string(b), value: c})
			}
		}
		if len(items) == 1 {
			return items[0].value
		}
		slices.SortFunc(items, func(a, b item) int { return strings.Compare(a.key, b.key) })
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item.value
		}
		return out
	case string:
		if principal {
			if m := rootARN.FindStringSubmatch(v); m != nil {
				return m[1]
			}
		}
		return v
	}
	return v
}

// withAWSPrincipals returns principals with its AWS principals in canonical form.
func withAWSPrincipals(principals map[string]any) map[string]any {
	out := make(map[string]any, len(principals))
	for key, value := range principals {
		if key == "AWS" {
			value = canonical(value, true)
		}
		out[key] = value
	}
	return out
}
