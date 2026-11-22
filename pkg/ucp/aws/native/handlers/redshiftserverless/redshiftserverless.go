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

// Package redshiftserverless has the native handlers for Amazon Redshift Serverless usage limits, managed VPC
// endpoints, and custom domain associations, which have no CloudFormation type.
//
// A change to a usage limit or a custom domain also changes the workgroup it belongs to, which reports MODIFYING until
// it is done, and Redshift Serverless rejects some changes while a workgroup is MODIFYING. So these handlers wait for
// the workgroup to be free before they finish, and a change AWS rejects because something is busy fails with 409, so
// the caller deploys again later.
package redshiftserverless

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// API is the part of the Redshift Serverless client the handlers use.
type API interface {
	CreateUsageLimit(ctx context.Context, params *redshiftserverless.CreateUsageLimitInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateUsageLimitOutput, error)
	GetUsageLimit(ctx context.Context, params *redshiftserverless.GetUsageLimitInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.GetUsageLimitOutput, error)
	UpdateUsageLimit(ctx context.Context, params *redshiftserverless.UpdateUsageLimitInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateUsageLimitOutput, error)
	DeleteUsageLimit(ctx context.Context, params *redshiftserverless.DeleteUsageLimitInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteUsageLimitOutput, error)
	ListUsageLimits(ctx context.Context, params *redshiftserverless.ListUsageLimitsInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.ListUsageLimitsOutput, error)

	CreateEndpointAccess(ctx context.Context, params *redshiftserverless.CreateEndpointAccessInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateEndpointAccessOutput, error)
	GetEndpointAccess(ctx context.Context, params *redshiftserverless.GetEndpointAccessInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.GetEndpointAccessOutput, error)
	UpdateEndpointAccess(ctx context.Context, params *redshiftserverless.UpdateEndpointAccessInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateEndpointAccessOutput, error)
	DeleteEndpointAccess(ctx context.Context, params *redshiftserverless.DeleteEndpointAccessInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteEndpointAccessOutput, error)
	ListEndpointAccess(ctx context.Context, params *redshiftserverless.ListEndpointAccessInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.ListEndpointAccessOutput, error)

	CreateCustomDomainAssociation(ctx context.Context, params *redshiftserverless.CreateCustomDomainAssociationInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateCustomDomainAssociationOutput, error)
	GetCustomDomainAssociation(ctx context.Context, params *redshiftserverless.GetCustomDomainAssociationInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.GetCustomDomainAssociationOutput, error)
	UpdateCustomDomainAssociation(ctx context.Context, params *redshiftserverless.UpdateCustomDomainAssociationInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.UpdateCustomDomainAssociationOutput, error)
	DeleteCustomDomainAssociation(ctx context.Context, params *redshiftserverless.DeleteCustomDomainAssociationInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.DeleteCustomDomainAssociationOutput, error)
	ListCustomDomainAssociations(ctx context.Context, params *redshiftserverless.ListCustomDomainAssociationsInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.ListCustomDomainAssociationsOutput, error)

	GetWorkgroup(ctx context.Context, params *redshiftserverless.GetWorkgroupInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.GetWorkgroupOutput, error)
	ListWorkgroups(ctx context.Context, params *redshiftserverless.ListWorkgroupsInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.ListWorkgroupsOutput, error)
}

var inRegion = awsapi.InRegion[redshiftserverless.Options]

// noRetry makes the SDK send a call once, for a create AWS would make twice if it got it twice.
func noRetry(o *redshiftserverless.Options) {
	o.RetryMaxAttempts = 1
}

const (
	// notFound is the code Redshift Serverless returns for a resource it doesn't have, and conflict the code of a
	// change it can't make now, such as one to a workgroup that is MODIFYING.
	notFound = "ResourceNotFoundException"
	conflict = "ConflictException"

	// validation is the code of a request whose values Redshift Serverless rejects.
	validation = "ValidationException"
)

// workgroupBusy are the workgroup states in which a change to it, or to what belongs to it, may still be running.
var workgroupBusy = []string{string(types.WorkgroupStatusCreating), string(types.WorkgroupStatusModifying)}

// busyFailure is the failure of a change AWS rejected with ConflictException, because the workgroup, or the resource
// itself, is busy with another change that ends on its own. UCP answers it with 409, and the caller deploys again.
func busyFailure(what string, err error) error {
	return &native.Failure{
		Message: fmt.Sprintf("%s is busy with another change, so AWS changed nothing. Deploy again once it is done, and give "+
			"resources of one Redshift Serverless workgroup dependsOn on each other: %s", what, err.Error()),
		ClientError: true,
		Conflict:    true,
		Outcome:     native.OutcomeRejected,
	}
}

// rejected returns err as a busy failure when AWS answered ConflictException, and as awsapi.Rejected does otherwise.
func rejected(what string, err error) error {
	if awsapi.HasCode(err, conflict) {
		return busyFailure(what, err)
	}
	return awsapi.Rejected(err)
}

// created returns what a Create whose start call failed with err returns: the identifier, unless AWS rejected the call
// and made nothing. A create AWS may have made, such as one whose answer was lost, keeps the identifier with its
// error, so a caller that cleans up what a create may have made, such as a failed recording, finds it.
func created(identifier string, err error) (string, error) {
	var failure *native.Failure
	if errors.As(err, &failure) {
		if failure.Outcome == native.OutcomeRejected {
			return "", err
		}
		return identifier, err
	}
	if ucp_aws.IsClientFault(err) {
		return "", err
	}
	return identifier, err
}

// unconfirmed is the failure of a create or an update AWS answered with ConflictException, err, when the read that
// would show whether an earlier attempt of it reached AWS failed with readErr. It may be applied, so its outcome is
// unknown, and UCP keeps the record and its values.
func unconfirmed(err error, readErr error) error {
	return &native.Failure{
		Message: fmt.Sprintf("AWS answered the change with %s, and reading the resource to see whether an earlier attempt "+
			"of it reached AWS failed: %s. Deploy again.", err.Error(), readErr.Error()),
		Outcome: native.OutcomeUnknown,
	}
}

// invalid is the failure of a template value the handler can't send, so it is never sent as an empty value or left
// out. It names the property and what it must be, never the value.
func invalid(typeName string, name string, want string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s of an %s must be %s", name, typeName, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// checkStrings checks that each named property the template sets is a string.
func checkStrings(typeName string, properties map[string]any, names ...string) error {
	for _, name := range names {
		if value, ok := properties[name]; ok && value != nil {
			if _, ok := value.(string); !ok {
				return invalid(typeName, name, "a string")
			}
		}
	}
	return nil
}

// stringList returns the list of strings name of properties, or nil when it is absent. A value that isn't a list of
// strings is a client failure, so it is never sent as a shorter list.
func stringList(typeName string, properties map[string]any, name string) ([]string, error) {
	value, ok := properties[name]
	if !ok || value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, invalid(typeName, name, "a list of strings")
	}
	out := []string{}
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, invalid(typeName, name, "a list of strings")
		}
		out = append(out, s)
	}
	return out, nil
}

// anyList returns list as a JSON list.
func anyList(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

// sameSet reports whether a and b hold the same strings, in any order.
func sameSet(a []string, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

// workgroupProgress returns the progress of a change that also changes the workgroup with the name: done once the
// workgroup is free, and waiting while it is busy. A workgroup AWS no longer has is free, because nothing of it is
// left to change.
func workgroupProgress(ctx context.Context, api API, region string, name string) (native.Progress, error) {
	out, err := api.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String(name)}, inRegion(region))
	if awsapi.HasCode(err, notFound) {
		return native.Progress{Done: true}, nil
	}
	if err != nil {
		return native.Progress{}, err
	}
	return statusProgress(out.Workgroup), nil
}

// statusProgress returns the progress of a change that also changes workgroup, as workgroupProgress does.
func statusProgress(workgroup *types.Workgroup) native.Progress {
	status := ""
	if workgroup != nil {
		status = string(workgroup.Status)
	}
	if slices.Contains(workgroupBusy, status) {
		return native.Progress{State: "workgroup " + status, Waiting: "workgroup " + string(types.WorkgroupStatusAvailable)}
	}
	return native.Progress{Done: true, State: status}
}
