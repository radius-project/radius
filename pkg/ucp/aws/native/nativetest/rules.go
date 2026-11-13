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

package nativetest

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

// awsDocs is the prefix of every page of AWS's documentation.
const awsDocs = "https://docs.aws.amazon.com/"

// Call is the record of one AWS call a handler makes: the call's page in AWS's API reference, and the rules AWS
// documents for it that the handler's fake enforces. See "Adding a native handler" in docs/ucp/aws.md.
type Call struct {
	// Doc is the call's page in AWS's API reference.
	Doc string

	// Rules are the rules AWS documents for the call: what AWS does when a parameter is left out, the state the
	// resource must be in, value limits, parameters that can't be combined, and the scope of an idempotency token.
	Rules []Rule

	// Unclear is what AWS's docs leave unclear about the call, which the fake doesn't enforce, or "".
	Unclear string
}

// Rule is a rule AWS documents for a call. Break is set for a rule that makes AWS reject a call, and Check for any
// other rule, such as a default AWS applies to a parameter left out.
type Rule struct {
	// Rule says what AWS documents, in plain words.
	Rule string

	// Doc is the AWS page that states the rule, when it isn't the call's page, such as the page of a data type the
	// call takes.
	Doc string

	// Break makes a call the rule forbids against a fake of the service's API backed by store, after any calls it
	// needs to set up, and returns the call's error. AWS rejects the call, so the fake must return a client fault
	// with the error code Code, the code AWS's docs give for it.
	Break func(ctx context.Context, store *awsfake.CloudControl) error
	Code  string

	// Check makes calls against a fake of the service's API backed by store that show the fake follows the rule, and
	// returns an error when it doesn't.
	Check func(ctx context.Context, store *awsfake.CloudControl) error
}

// requireAPIRules checks the record of the AWS calls the handler makes, read from its package's source, with
// requireRecorded and requireRules.
func requireAPIRules(t testing.TB, api map[string]Call, handler any) {
	t.Helper()
	calls, typeName := handlerCalls(t, handler)
	requireRecorded(t, api, calls, typeName)
	requireRules(t, api)
}

// requireRecorded checks that every call the handler makes has a record, and every record is for such a call.
func requireRecorded(t testing.TB, api map[string]Call, calls []string, typeName string) {
	t.Helper()
	missing := []string{}
	for _, call := range calls {
		if _, ok := api[call]; !ok {
			missing = append(missing, call)
		}
	}
	require.Empty(t, missing, "record each AWS call %s makes in Suite.API, with its page in AWS's API reference and the rules the page states", typeName)
	extra := []string{}
	for call := range api {
		if !slices.Contains(calls, call) {
			extra = append(extra, call)
		}
	}
	slices.Sort(extra)
	require.Empty(t, extra, "Suite.API records calls %s doesn't make", typeName)
}

// requireRules checks that each record links to AWS's docs, and that the fake follows every rule: it rejects each
// call a rule forbids with a client fault that has the error code AWS documents, and passes each rule's Check.
func requireRules(t testing.TB, api map[string]Call) {
	t.Helper()
	names := slices.Sorted(maps.Keys(api))
	for _, name := range names {
		call := api[name]
		require.Truef(t, strings.HasPrefix(call.Doc, awsDocs), "%s: Doc must link to the call's page in AWS's API reference, not %q", name, call.Doc)
		for _, rule := range call.Rules {
			where := name + ": " + rule.Rule
			require.NotEmpty(t, rule.Rule, "%s: a rule must say what AWS documents", name)
			require.Truef(t, rule.Doc == "" || strings.HasPrefix(rule.Doc, awsDocs), "%s: Doc must link to AWS's docs, not %q", where, rule.Doc)
			require.Truef(t, (rule.Break == nil) != (rule.Check == nil), "%s: set either Break or Check", where)
			ctx := t.Context()
			if rule.Check != nil {
				require.NoError(t, rule.Check(ctx, awsfake.NewCloudControl()), "%s: the fake doesn't follow the rule", where)
				continue
			}
			require.NotEmpty(t, rule.Code, "%s: Code must be the error code AWS's docs give for the call Break makes", where)
			err := rule.Break(ctx, awsfake.NewCloudControl())
			require.Errorf(t, err, "%s: the fake accepts a call AWS rejects", where)
			var apiErr smithy.APIError
			require.Truef(t, errors.As(err, &apiErr) && apiErr.ErrorFault() == smithy.FaultClient, "%s: the fake must reject the call with a client fault, as AWS does, not %v", where, err)
			require.Equalf(t, rule.Code, apiErr.ErrorCode(), "%s: the fake rejects the call for another reason: %v", where, err)
		}
	}
}
