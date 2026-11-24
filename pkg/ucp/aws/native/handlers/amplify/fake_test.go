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
	"regexp"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/amplify/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	// account and region are the account and region of the fake's caller.
	account = "123456789012"
	region  = "us-west-2"

	// gen1App is a Gen 1 app, whose backend environments the API serves, and gen2App a Gen 2 app, whose it doesn't.
	gen1App = "d1a2b3c4d5e6f7"
	gen2App = "d9z8y7x6w5v4u3"

	// badRequestCode is the error each call lists for a request with unexpected data, which the fake returns for a
	// value outside its documented length or pattern.
	badRequestCode = "BadRequestException"
	notFoundCode   = "NotFoundException"
)

// appIDPattern is the documented pattern of an app ID.
var appIDPattern = regexp.MustCompile(`^d[a-z0-9]+$`)

// fakeAPI is an in-memory AWS Amplify API for backend environments. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. It follows the rules in
// backendEnvironmentAPI.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// validLength reports whether v is min to max characters, or is nil when optional.
func validLength(v *string, min int, max int, optional bool) bool {
	if v == nil {
		return optional
	}
	n := utf8.RuneCountInString(*v)
	return n >= min && n <= max
}

// app returns the app the call names, or the error AWS returns for it.
func app(id *string) error {
	if !validLength(id, 1, 20, false) || !appIDPattern.MatchString(*id) {
		return fault(badRequestCode)
	}
	switch *id {
	case gen1App:
		return nil
	case gen2App:
		return fault(badRequestCode)
	}
	return fault(notFoundCode)
}

// environment returns the stored environment the call names, or nil.
func (f *fakeAPI) environment(app *string, name *string) map[string]any {
	return f.store.Resource(backendEnvironmentType, aws.ToString(app)+"|"+aws.ToString(name))
}

// output returns the environment as AWS returns it. The suite's Cloud Control can store one without the values AWS
// sets, so each is read only when it is there.
func output(stored map[string]any) *types.BackendEnvironment {
	now := time.Unix(1700000000, 0)
	env := &types.BackendEnvironment{CreateTime: &now, UpdateTime: &now}
	for name, field := range map[string]**string{
		"BackendEnvironmentArn": &env.BackendEnvironmentArn,
		"EnvironmentName":       &env.EnvironmentName,
		"StackName":             &env.StackName,
		"DeploymentArtifacts":   &env.DeploymentArtifacts,
	} {
		if v, ok := stored[name].(string); ok {
			*field = aws.String(v)
		}
	}
	return env
}

func (f *fakeAPI) CreateBackendEnvironment(ctx context.Context, in *amplify.CreateBackendEnvironmentInput, _ ...func(*amplify.Options)) (*amplify.CreateBackendEnvironmentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreateBackendEnvironment"]; err != nil {
		return nil, err
	}
	if !validLength(in.EnvironmentName, 1, 255, false) || !validLength(in.StackName, 1, 255, true) || !validLength(in.DeploymentArtifacts, 1, 1000, true) {
		return nil, fault(badRequestCode)
	}
	if err := app(in.AppId); err != nil {
		return nil, err
	}
	if f.environment(in.AppId, in.EnvironmentName) != nil {
		return nil, fault(badRequestCode)
	}
	id := aws.ToString(in.AppId) + "|" + aws.ToString(in.EnvironmentName)
	stored := map[string]any{
		"AppId":                 aws.ToString(in.AppId),
		"EnvironmentName":       aws.ToString(in.EnvironmentName),
		"BackendEnvironmentArn": "arn:aws:amplify:" + region + ":" + account + ":apps/" + aws.ToString(in.AppId) + "/backendenvironments/" + aws.ToString(in.EnvironmentName),
	}
	if in.StackName != nil {
		stored["StackName"] = *in.StackName
	}
	if in.DeploymentArtifacts != nil {
		stored["DeploymentArtifacts"] = *in.DeploymentArtifacts
	}
	f.store.Put(backendEnvironmentType, id, stored)
	return &amplify.CreateBackendEnvironmentOutput{BackendEnvironment: output(stored)}, nil
}

func (f *fakeAPI) GetBackendEnvironment(ctx context.Context, in *amplify.GetBackendEnvironmentInput, _ ...func(*amplify.Options)) (*amplify.GetBackendEnvironmentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["GetBackendEnvironment"]; err != nil {
		return nil, err
	}
	if !validLength(in.EnvironmentName, 1, 255, false) {
		return nil, fault(badRequestCode)
	}
	if err := app(in.AppId); err != nil {
		return nil, err
	}
	stored := f.environment(in.AppId, in.EnvironmentName)
	if stored == nil {
		return nil, fault(notFoundCode)
	}
	return &amplify.GetBackendEnvironmentOutput{BackendEnvironment: output(stored)}, nil
}

func (f *fakeAPI) DeleteBackendEnvironment(ctx context.Context, in *amplify.DeleteBackendEnvironmentInput, _ ...func(*amplify.Options)) (*amplify.DeleteBackendEnvironmentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DeleteBackendEnvironment"]; err != nil {
		return nil, err
	}
	if !validLength(in.EnvironmentName, 1, 255, false) {
		return nil, fault(badRequestCode)
	}
	if err := app(in.AppId); err != nil {
		return nil, err
	}
	stored := f.environment(in.AppId, in.EnvironmentName)
	if stored == nil {
		return nil, fault(notFoundCode)
	}
	f.store.Remove(backendEnvironmentType, aws.ToString(in.AppId)+"|"+aws.ToString(in.EnvironmentName))
	return &amplify.DeleteBackendEnvironmentOutput{BackendEnvironment: output(stored)}, nil
}
