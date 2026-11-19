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

// Package amplify has the native handler for AWS Amplify backend environments, which have no CloudFormation type.
package amplify

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/amplify/types"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const backendEnvironmentType = "AWS::Amplify::BackendEnvironment"

// API is the part of the AWS Amplify client the handler uses.
type API interface {
	CreateBackendEnvironment(ctx context.Context, params *amplify.CreateBackendEnvironmentInput, optFns ...func(*amplify.Options)) (*amplify.CreateBackendEnvironmentOutput, error)
	GetBackendEnvironment(ctx context.Context, params *amplify.GetBackendEnvironmentInput, optFns ...func(*amplify.Options)) (*amplify.GetBackendEnvironmentOutput, error)
	DeleteBackendEnvironment(ctx context.Context, params *amplify.DeleteBackendEnvironmentInput, optFns ...func(*amplify.Options)) (*amplify.DeleteBackendEnvironmentOutput, error)
}

var inRegion = awsapi.InRegion[amplify.Options]

var (
	_ native.Handler           = (*BackendEnvironment)(nil)
	_ native.IdentifierEncoder = (*BackendEnvironment)(nil)
)

// BackendEnvironment serves AWS::Amplify::BackendEnvironment: a backend environment of an Amplify Gen 1 app. Its
// identifier is AppId and EnvironmentName joined with "|". AWS has no call that changes an environment, so every
// property is create-only, and UCP rejects a change with 400 before any AWS call.
type BackendEnvironment struct {
	api API
}

// NewBackendEnvironment returns the handler for Amplify backend environments.
func NewBackendEnvironment(api API) *BackendEnvironment {
	return &BackendEnvironment{api: api}
}

// TypeName returns AWS::Amplify::BackendEnvironment.
func (h *BackendEnvironment) TypeName() string {
	return backendEnvironmentType
}

// Options returns how UCP uses the handler. AWS lists environments only by app, so the type can't be listed.
func (h *BackendEnvironment) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// environment is a backend environment as a template gives it.
type environment struct {
	app, name, stack, artifacts string
}

func (e environment) id() string {
	return e.app + "|" + e.name
}

// clientFailure is a failure for a value AWS would reject or can't take, which UCP reports with 400.
func clientFailure(format string, args ...any) error {
	return &native.Failure{Message: fmt.Sprintf(format, args...), ClientError: true, Outcome: native.OutcomeRejected}
}

// parse returns the environment the properties give. A value that isn't a string, or an app ID or name with "|",
// which separates them in the identifier, is a client failure.
func parse(properties map[string]any) (environment, error) {
	e := environment{}
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"AppId", &e.app},
		{"EnvironmentName", &e.name},
		{"StackName", &e.stack},
		{"DeploymentArtifacts", &e.artifacts},
	} {
		v, ok := properties[field.name]
		if !ok || v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return environment{}, clientFailure("%s of an %s must be a string", field.name, backendEnvironmentType)
		}
		*field.value = s
	}
	if strings.Contains(e.app, "|") || strings.Contains(e.name, "|") {
		return environment{}, clientFailure("AppId and EnvironmentName of an %s can't contain \"|\", which Radius uses to join them into the environment's identifier", backendEnvironmentType)
	}
	return e, nil
}

// parseID returns the app ID and environment name an identifier names.
func parseID(identifier string) (environment, error) {
	app, name, ok := strings.Cut(identifier, "|")
	if !ok || app == "" || name == "" || strings.Contains(name, "|") {
		return environment{}, clientFailure("%q is not the identifier of an AWS Amplify backend environment: it must be AppId|EnvironmentName", identifier)
	}
	return environment{app: app, name: name}, nil
}

// Identifier returns AppId|EnvironmentName, or "" when the properties don't give both. UCP calls it before any AWS
// call, so a value of the wrong type is rejected here.
func (h *BackendEnvironment) Identifier(properties map[string]any) (string, error) {
	e, err := parse(properties)
	if err != nil || e.app == "" || e.name == "" {
		return "", err
	}
	return e.id(), nil
}

// get returns the environment, or ErrNotFound when the app, or the environment in it, doesn't exist.
func (h *BackendEnvironment) get(ctx context.Context, region string, e environment) (*types.BackendEnvironment, error) {
	out, err := h.api.GetBackendEnvironment(ctx, &amplify.GetBackendEnvironmentInput{AppId: aws.String(e.app), EnvironmentName: aws.String(e.name)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "NotFoundException")
	}
	return out.BackendEnvironment, nil
}

// Read returns the environment's properties.
func (h *BackendEnvironment) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	e, err := parseID(identifier)
	if err != nil {
		return nil, err
	}
	env, err := h.get(ctx, region, e)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{
		"AppId":                 e.app,
		"EnvironmentName":       e.name,
		"BackendEnvironmentArn": aws.ToString(env.BackendEnvironmentArn),
	}
	if v := aws.ToString(env.StackName); v != "" {
		properties["StackName"] = v
	}
	if v := aws.ToString(env.DeploymentArtifacts); v != "" {
		properties["DeploymentArtifacts"] = v
	}
	return properties, nil
}

// Create creates the environment. AWS creates it in the call.
func (h *BackendEnvironment) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	e, err := parse(request.Properties)
	if err != nil {
		return "", err
	}
	if e.app == "" || e.name == "" {
		return "", clientFailure("an %s needs AppId and EnvironmentName", backendEnvironmentType)
	}
	_, err = h.api.CreateBackendEnvironment(ctx, &amplify.CreateBackendEnvironmentInput{
		AppId:               aws.String(e.app),
		EnvironmentName:     aws.String(e.name),
		StackName:           optional(e.stack),
		DeploymentArtifacts: optional(e.artifacts),
	}, inRegion(request.Region))
	if ucp_aws.IsClientFault(err) {
		// UCP reads AWS by the identifier before it creates an environment, so one AWS has now may be the one an
		// earlier attempt of this create made, such as when the SDK retried a call whose response was lost. The docs
		// name no error for a name in use, so any rejection is checked. The outcome is then unknown, so UCP keeps the
		// record and the next deploy finds the environment by its identifier.
		if _, getErr := h.get(ctx, request.Region, e); getErr == nil {
			return "", &native.Failure{
				Message:     fmt.Sprintf("the Amplify app %q already has a backend environment named %q, which an earlier attempt of this deploy may have created. Deploy again to manage it, or choose another name: %v", e.app, e.name, err),
				ClientError: true,
				Outcome:     native.OutcomeUnknown,
			}
		}
	}
	if err != nil {
		return "", err
	}
	return e.id(), nil
}

// optional returns nil for "", so a property the template leaves out isn't sent.
func optional(v string) *string {
	if v == "" {
		return nil
	}
	return aws.String(v)
}

// Update changes nothing, because every property is create-only.
func (h *BackendEnvironment) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete deletes the environment.
func (h *BackendEnvironment) Delete(ctx context.Context, region string, identifier string) error {
	e, err := parseID(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteBackendEnvironment(ctx, &amplify.DeleteBackendEnvironmentInput{AppId: aws.String(e.app), EnvironmentName: aws.String(e.name)}, inRegion(region))
	return awsapi.NotFound(err, "NotFoundException")
}

// List returns nothing, because AWS lists backend environments only by app.
func (h *BackendEnvironment) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the environment. AWS creates and deletes an environment in the call, so a create is done once AWS
// has it, and a delete once it doesn't. An update changes nothing, so it is done.
func (h *BackendEnvironment) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	e, err := parseID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.get(ctx, region, e); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true}, nil
}
