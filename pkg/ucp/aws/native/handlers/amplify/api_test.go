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

package amplify

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/amplify"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference   = "https://docs.aws.amazon.com/amplify/latest/APIReference/"
	environmentDoc = apiReference + "API_BackendEnvironment.html"

	// gen1Note is what each call's page says about Gen 2 apps, and leaves unclear.
	gen1Note = " The page says the call is available only to Gen 1 apps, without naming the error for a Gen 2 app; " +
		"the fake returns BadRequestException."

	// constraintNote is what every call's page leaves unclear about its constraints.
	constraintNote = " The page names no error for a value outside its length or pattern; the fake returns " +
		"BadRequestException, which the page lists for a request with unexpected data."
)

// apiCall is a call against a fake Amplify API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake Amplify API backed by store. It
// stops at the first call that fails, and returns its error.
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

// createEnvironment creates the environment staging of the Gen 1 app, with the changes edits make to the input.
func createEnvironment(edits ...func(*amplify.CreateBackendEnvironmentInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &amplify.CreateBackendEnvironmentInput{
			AppId:           aws.String(gen1App),
			EnvironmentName: aws.String("staging"),
			StackName:       aws.String("amplify-app-staging"),
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreateBackendEnvironment(ctx, in)
		return err
	}
}

// inApp returns an edit that creates the environment in the app id.
func inApp(id string) func(*amplify.CreateBackendEnvironmentInput) {
	return func(in *amplify.CreateBackendEnvironmentInput) { in.AppId = aws.String(id) }
}

// getEnvironment reads the environment name of the app id.
func getEnvironment(id string, name string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.GetBackendEnvironment(ctx, &amplify.GetBackendEnvironmentInput{AppId: aws.String(id), EnvironmentName: aws.String(name)})
		return err
	}
}

// deleteEnvironment deletes the environment name of the app id.
func deleteEnvironment(id string, name string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		_, err := f.DeleteBackendEnvironment(ctx, &amplify.DeleteBackendEnvironmentInput{AppId: aws.String(id), EnvironmentName: aws.String(name)})
		return err
	}
}

// backendEnvironmentAPI records the Amplify calls BackendEnvironment makes.
var backendEnvironmentAPI = map[string]nativetest.Call{
	"CreateBackendEnvironment": {
		Doc: apiReference + "API_CreateBackendEnvironment.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "appId is 1 to 20 characters",
				Break: against(createEnvironment(inApp("d" + strings.Repeat("a", 20)))),
				Code:  badRequestCode,
			},
			{
				Rule:  "appId matches d[a-z0-9]+",
				Break: against(createEnvironment(inApp("D1A2B3"))),
				Code:  badRequestCode,
			},
			{
				Rule:  "environmentName is required, and 1 to 255 characters",
				Break: against(createEnvironment(func(in *amplify.CreateBackendEnvironmentInput) { in.EnvironmentName = aws.String("") })),
				Code:  badRequestCode,
			},
			{
				Rule:  "stackName is 1 to 255 characters",
				Break: against(createEnvironment(func(in *amplify.CreateBackendEnvironmentInput) { in.StackName = aws.String(strings.Repeat("s", 256)) })),
				Code:  badRequestCode,
			},
			{
				Rule: "deploymentArtifacts is 1 to 1000 characters",
				Break: against(createEnvironment(func(in *amplify.CreateBackendEnvironmentInput) {
					in.DeploymentArtifacts = aws.String(strings.Repeat("a", 1001))
				})),
				Code: badRequestCode,
			},
			{
				Rule:  "the app exists",
				Break: against(createEnvironment(inApp("d0000000000000"))),
				Code:  notFoundCode,
			},
			{
				Rule:  "the call is available only to Gen 1 apps",
				Break: against(createEnvironment(inApp(gen2App))),
				Code:  badRequestCode,
			},
			{
				Rule:  "the app has no environment with the name",
				Break: against(createEnvironment(), createEnvironment()),
				Code:  badRequestCode,
			},
			{
				Rule: "stackName and deploymentArtifacts are optional, and the environment has an ARN",
				Doc:  environmentDoc,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					out, err := f.CreateBackendEnvironment(ctx, &amplify.CreateBackendEnvironmentInput{AppId: aws.String(gen1App), EnvironmentName: aws.String("dev")})
					if err != nil {
						return err
					}
					env := out.BackendEnvironment
					if env.StackName != nil || env.DeploymentArtifacts != nil || aws.ToString(env.BackendEnvironmentArn) == "" {
						return fmt.Errorf("the environment is %+v", env)
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't name the error for a name the app already has; the fake returns BadRequestException. " +
			"The handler reads the environment before it creates one, and reports a rejected create of an environment " +
			"AWS then has as an unknown outcome, because an SDK retry of a create whose response was lost gets it. The " +
			"page doesn't say whether AWS fills in stackName or deploymentArtifacts when they are left out; the fake " +
			"leaves them out. It doesn't say whether the call creates the CloudFormation stack, or only records its " +
			"name; the fake only records it. It doesn't give the ARN's format; the fake uses " +
			"arn:aws:amplify:<region>:<account>:apps/<appId>/backendenvironments/<name>." + gen1Note + constraintNote,
	},
	"GetBackendEnvironment": {
		Doc: apiReference + "API_GetBackendEnvironment.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the environment exists",
				Break: against(getEnvironment(gen1App, "missing")),
				Code:  notFoundCode,
			},
			{
				Rule:  "the app exists",
				Break: against(getEnvironment("d0000000000000", "staging")),
				Code:  notFoundCode,
			},
			{
				Rule:  "environmentName is 1 to 255 characters",
				Break: against(getEnvironment(gen1App, strings.Repeat("e", 256))),
				Code:  badRequestCode,
			},
			{
				Rule: "the environment has the stackName and deploymentArtifacts it was created with",
				Doc:  environmentDoc,
				Check: against(createEnvironment(func(in *amplify.CreateBackendEnvironmentInput) {
					in.DeploymentArtifacts = aws.String("amplify-app-deployment")
				}), func(ctx context.Context, f *fakeAPI) error {
					out, err := f.GetBackendEnvironment(ctx, &amplify.GetBackendEnvironmentInput{AppId: aws.String(gen1App), EnvironmentName: aws.String("staging")})
					if err != nil {
						return err
					}
					env := out.BackendEnvironment
					if aws.ToString(env.StackName) != "amplify-app-staging" || aws.ToString(env.DeploymentArtifacts) != "amplify-app-deployment" {
						return fmt.Errorf("the environment is %+v", env)
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't say whether a missing app and a missing environment get the same error; the fake " +
			"returns NotFoundException for both, and the handler treats both as not found." + gen1Note + constraintNote,
	},
	"DeleteBackendEnvironment": {
		Doc: apiReference + "API_DeleteBackendEnvironment.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the environment exists",
				Break: against(deleteEnvironment(gen1App, "staging")),
				Code:  notFoundCode,
			},
			{
				Rule:  "environmentName is 1 to 255 characters",
				Break: against(deleteEnvironment(gen1App, "")),
				Code:  badRequestCode,
			},
			{
				Rule: "a deleted environment is gone",
				Check: against(createEnvironment(), deleteEnvironment(gen1App, "staging"), func(ctx context.Context, f *fakeAPI) error {
					if err := getEnvironment(gen1App, "staging")(ctx, f); err == nil {
						return fmt.Errorf("the fake still has the deleted environment")
					}
					return nil
				}),
			},
		},
		Unclear: "The page doesn't say whether the call deletes the environment's CloudFormation stack, or whether the " +
			"environment can still be read for a while after the call; the fake removes the environment at once and " +
			"has no stack, and the handler waits until a read gets NotFoundException. It lists " +
			"DependentServiceFailureException without saying when; the fake never returns it." + gen1Note + constraintNote,
	},
}
