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

package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference  = "https://docs.aws.amazon.com/lambda/latest/microvm-api/"
	idlePolicyDoc = apiReference + "API_IdlePolicy.html"
	cloudWatchDoc = apiReference + "API_CloudWatchLogging.html"
	// tokenDoc is the page of the same API that says what a client token does, which RunMicrovm's page doesn't.
	tokenDoc       = apiReference + "API_CreateMicrovmImage.html"
	gettingStarted = "https://docs.aws.amazon.com/lambda/latest/dg/microvms-getting-started.html"
	roleARN        = "arn:aws:iam::" + account + ":role/microvm-execution"
)

// apiCall is a call against a fake Lambda MicroVMs API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake Lambda MicroVMs API backed by
// store. It stops at the first call that fails, and returns its error.
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

// runInput returns the input that runs the image by name, with the changes edits make.
func runInput(edits ...func(*lambdamicrovms.RunMicrovmInput)) *lambdamicrovms.RunMicrovmInput {
	in := &lambdamicrovms.RunMicrovmInput{ImageIdentifier: aws.String(image), ClientToken: aws.String("token-1")}
	for _, edit := range edits {
		edit(in)
	}
	return in
}

// run runs a MicroVM with runInput(edits...).
func run(edits ...func(*lambdamicrovms.RunMicrovmInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.RunMicrovm(ctx, runInput(edits...))
		return err
	}
}

// states returns the state of the first MicroVM the fake ran on each of n reads.
func states(ctx context.Context, f *fakeAPI, n int) ([]types.MicrovmState, error) {
	out := []types.MicrovmState{}
	for range n {
		got, err := f.GetMicrovm(ctx, &lambdamicrovms.GetMicrovmInput{MicrovmIdentifier: aws.String(firstID)})
		if err != nil {
			return nil, err
		}
		out = append(out, got.State)
	}
	return out, nil
}

// firstID is the ID of the first MicroVM a fake runs.
const firstID = "mvm-00000001-0000-0000-0000-000000000000"

// expectStates returns an error unless got is want.
func expectStates(got []types.MicrovmState, want ...types.MicrovmState) error {
	if fmt.Sprint(got) != fmt.Sprint(want) {
		return fmt.Errorf("the states are %v, not %v", got, want)
	}
	return nil
}

func terminate(id string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.TerminateMicrovm(ctx, &lambdamicrovms.TerminateMicrovmInput{MicrovmIdentifier: aws.String(id)})
		return err
	}
}

// microvmAPI records the Lambda MicroVMs calls Microvm makes.
var microvmAPI = map[string]nativetest.Call{
	"RunMicrovm": {
		Doc: apiReference + "API_RunMicrovm.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "imageIdentifier is required, and 1 to 256 characters",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.ImageIdentifier = aws.String(strings.Repeat("i", 257)) })),
				Code:  validationCode,
			},
			{
				Rule:  "the image exists",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.ImageIdentifier = aws.String("missing") })),
				Code:  notFoundCode,
			},
			{
				Rule:  "clientToken is 1 to 128 characters",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.ClientToken = aws.String(strings.Repeat("t", 129)) })),
				Code:  validationCode,
			},
			{
				Rule:  "imageVersion has no whitespace",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.ImageVersion = aws.String("1 2") })),
				Code:  validationCode,
			},
			{
				Rule: "executionRoleArn is an IAM role ARN",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.ExecutionRoleArn = aws.String("arn:aws:iam::123:user/someone")
				})),
				Code: validationCode,
			},
			{
				Rule: "ingressNetworkConnectors and egressNetworkConnectors have at most 10 items",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.EgressNetworkConnectors = strings.Split(strings.Repeat("c,", 11)[:21], ",")
				})),
				Code: validationCode,
			},
			{
				Rule:  "a network connector is 1 to 2048 characters",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.IngressNetworkConnectors = []string{""} })),
				Code:  validationCode,
			},
			{
				Rule:  "maximumDurationInSeconds is 1 to 28800",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.MaximumDurationInSeconds = aws.Int32(28801) })),
				Code:  validationCode,
			},
			{
				Rule:  "runHookPayload is at most 4096 characters",
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) { in.RunHookPayload = aws.String(strings.Repeat("p", 4097)) })),
				Code:  validationCode,
			},
			{
				Rule: "an idle policy sets all three of its fields",
				Doc:  idlePolicyDoc,
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.IdlePolicy = &types.IdlePolicy{AutoResumeEnabled: aws.Bool(true), MaxIdleDurationSeconds: aws.Int32(900)}
				})),
				Code: validationCode,
			},
			{
				Rule: "maxIdleDurationSeconds is at least 60",
				Doc:  idlePolicyDoc,
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.IdlePolicy = &types.IdlePolicy{AutoResumeEnabled: aws.Bool(true), MaxIdleDurationSeconds: aws.Int32(59), SuspendedDurationSeconds: aws.Int32(0)}
				})),
				Code: validationCode,
			},
			{
				Rule: "suspendedDurationSeconds is at least 0",
				Doc:  idlePolicyDoc,
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.IdlePolicy = &types.IdlePolicy{AutoResumeEnabled: aws.Bool(false), MaxIdleDurationSeconds: aws.Int32(60), SuspendedDurationSeconds: aws.Int32(-1)}
				})),
				Code: validationCode,
			},
			{
				Rule: "a CloudWatch log group is 1 to 512 characters of letters, digits, and _-/.#",
				Doc:  cloudWatchDoc,
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.Logging = &types.LoggingMemberCloudWatch{Value: types.CloudWatchLogging{LogGroup: aws.String("logs:bad")}}
				})),
				Code: validationCode,
			},
			{
				Rule: "a CloudWatch log stream has no : or *",
				Doc:  cloudWatchDoc,
				Break: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.Logging = &types.LoggingMemberCloudWatch{Value: types.CloudWatchLogging{LogGroup: aws.String("/app"), LogStream: aws.String("a*b")}}
				})),
				Code: validationCode,
			},
			{
				Rule: "a new MicroVM is PENDING, then RUNNING",
				Check: against(run(), func(ctx context.Context, f *fakeAPI) error {
					got, err := states(ctx, f, 2)
					if err != nil {
						return err
					}
					return expectStates(got, types.MicrovmStatePending, types.MicrovmStateRunning)
				}),
			},
			{
				Rule: "an image named by its name runs the image, which AWS reports by its ARN, at its latest version when imageVersion is left out",
				Doc:  gettingStarted,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					out, err := f.RunMicrovm(ctx, runInput())
					if err != nil {
						return err
					}
					if aws.ToString(out.ImageArn) != imageArn || aws.ToString(out.ImageVersion) != latestVersion {
						return fmt.Errorf("the MicroVM runs %s version %s", aws.ToString(out.ImageArn), aws.ToString(out.ImageVersion))
					}
					return nil
				}),
			},
			{
				Rule: "a retry with the same client token returns the first MicroVM, and runs no other",
				Doc:  tokenDoc,
				Check: against(run(), func(ctx context.Context, f *fakeAPI) error {
					out, err := f.RunMicrovm(ctx, runInput(func(in *lambdamicrovms.RunMicrovmInput) { in.ImageVersion = aws.String("1") }))
					if err != nil {
						return err
					}
					if aws.ToString(out.MicrovmId) != firstID || f.runs != 1 {
						return fmt.Errorf("the retry returned %s, after %d runs", aws.ToString(out.MicrovmId), f.runs)
					}
					return nil
				}),
			},
		},
		Unclear: "RunMicrovm's page says clientToken ensures idempotency without saying what a retry gets; the " +
			"CreateMicrovmImage page of the same API says a retry with the same token returns the successful " +
			"response without doing anything more. Neither says what AWS does when the retry has other values, or how " +
			"long AWS keeps a token; the fake returns the first MicroVM, with its values, for as long as it lives. The " +
			"page doesn't say what AWS defaults maximumDurationInSeconds or idlePolicy to when they are left out; the " +
			"fake reports neither. It doesn't name the error for an image version that doesn't exist; the fake returns " +
			"ResourceNotFoundException. Its runHookPayload description says 16,384 bytes, but its constraint says 4096 " +
			"characters; the fake takes 4096. It doesn't say whether a new MicroVM can terminate before it is RUNNING, " +
			"such as when the application fails to start; the fake can, for the handler's tests. It doesn't give the " +
			"endpoint's form; the fake makes one up. Logging is a union, so the SDK sends one kind at most; the handler " +
			"rejects a template that sets both or neither.",
	},
	"GetMicrovm": {
		Doc: apiReference + "API_GetMicrovm.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the MicroVM exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetMicrovm(ctx, &lambdamicrovms.GetMicrovmInput{MicrovmIdentifier: aws.String("mvm-missing")})
					return err
				}),
				Code: notFoundCode,
			},
			{
				Rule: "microvmIdentifier is 1 to 256 characters",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetMicrovm(ctx, &lambdamicrovms.GetMicrovmInput{MicrovmIdentifier: aws.String(strings.Repeat("m", 257))})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "the MicroVM has the role, idle policy, network connectors, and maximum duration it was run with",
				Check: against(run(func(in *lambdamicrovms.RunMicrovmInput) {
					in.ExecutionRoleArn = aws.String(roleARN)
					in.IdlePolicy = &types.IdlePolicy{AutoResumeEnabled: aws.Bool(true), MaxIdleDurationSeconds: aws.Int32(900), SuspendedDurationSeconds: aws.Int32(300)}
					in.IngressNetworkConnectors = []string{"ingress"}
					in.MaximumDurationInSeconds = aws.Int32(3600)
				}), func(ctx context.Context, f *fakeAPI) error {
					out, err := f.GetMicrovm(ctx, &lambdamicrovms.GetMicrovmInput{MicrovmIdentifier: aws.String(firstID)})
					if err != nil {
						return err
					}
					if aws.ToString(out.ExecutionRoleArn) != roleARN || aws.ToInt32(out.IdlePolicy.SuspendedDurationSeconds) != 300 ||
						fmt.Sprint(out.IngressNetworkConnectors) != "[ingress]" || aws.ToInt32(out.MaximumDurationInSeconds) != 3600 {
						return fmt.Errorf("the MicroVM is %+v", out)
					}
					return nil
				}),
			},
		},
		Unclear: "The page says the state is eventually consistent, without saying for how long; the fake reports each " +
			"state change on the next read. It doesn't say how long AWS returns a terminated MicroVM; the fake returns " +
			"it as TERMINATED for as long as it lives, and the handler reads a TERMINATING or TERMINATED MicroVM as not " +
			"found. It returns neither logging nor runHookPayload, so the schema makes them write-only.",
	},
	"TerminateMicrovm": {
		Doc: apiReference + "API_TerminateMicrovm.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the MicroVM exists",
				Break: against(terminate("mvm-missing")),
				Code:  notFoundCode,
			},
			{
				Rule:  "microvmIdentifier is 1 to 256 characters",
				Break: against(terminate("")),
				Code:  validationCode,
			},
			{
				Rule: "a terminated MicroVM is TERMINATING, then TERMINATED",
				Check: against(run(), terminate(firstID), func(ctx context.Context, f *fakeAPI) error {
					got, err := states(ctx, f, 2)
					if err != nil {
						return err
					}
					return expectStates(got, types.MicrovmStateTerminating, types.MicrovmStateTerminated)
				}),
			},
			{
				Rule:  "terminating a MicroVM AWS already terminated succeeds",
				Check: against(run(), terminate(firstID), terminate(firstID)),
			},
		},
		Unclear: "The page lists ConflictException without saying when, such as for a MicroVM that is still PENDING; " +
			"the fake never returns it, and the handler returns it as AWS's error.",
	},
	"ListMicrovms": {
		Doc: apiReference + "API_ListMicrovms.html",
		Rules: []nativetest.Rule{
			{
				Rule: "maxResults is 1 to 50",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.ListMicrovms(ctx, &lambdamicrovms.ListMicrovmsInput{MaxResults: aws.Int32(51)})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "a page with more after it has a nextToken that gets the rest",
				Check: against(run(), run(func(in *lambdamicrovms.RunMicrovmInput) { in.ClientToken = aws.String("token-2") }),
					func(ctx context.Context, f *fakeAPI) error {
						first, err := f.ListMicrovms(ctx, &lambdamicrovms.ListMicrovmsInput{MaxResults: aws.Int32(1)})
						if err != nil {
							return err
						}
						rest, err := f.ListMicrovms(ctx, &lambdamicrovms.ListMicrovmsInput{MaxResults: aws.Int32(1), NextToken: first.NextToken})
						if err != nil {
							return err
						}
						if len(first.Items) != 1 || len(rest.Items) != 1 || rest.NextToken != nil {
							return fmt.Errorf("the pages have %d and %d MicroVMs", len(first.Items), len(rest.Items))
						}
						return nil
					}),
			},
		},
		Unclear: "The page doesn't say whether it lists terminated MicroVMs, or its default page size; the fake lists " +
			"them, and the handler leaves out those AWS is terminating or has terminated. The fake has 2 MicroVMs a page.",
	},
}
