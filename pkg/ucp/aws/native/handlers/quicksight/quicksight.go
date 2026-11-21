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

// Package quicksight has the native handlers for Amazon Quick Sight resources that have no CloudFormation type: an
// asset's membership in a folder, and a template alias.
package quicksight

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/quicksight"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// API is the part of the Amazon Quick Sight client the handlers use.
type API interface {
	CreateFolderMembership(ctx context.Context, params *quicksight.CreateFolderMembershipInput, optFns ...func(*quicksight.Options)) (*quicksight.CreateFolderMembershipOutput, error)
	DeleteFolderMembership(ctx context.Context, params *quicksight.DeleteFolderMembershipInput, optFns ...func(*quicksight.Options)) (*quicksight.DeleteFolderMembershipOutput, error)
	ListFolderMembers(ctx context.Context, params *quicksight.ListFolderMembersInput, optFns ...func(*quicksight.Options)) (*quicksight.ListFolderMembersOutput, error)
	CreateTemplateAlias(ctx context.Context, params *quicksight.CreateTemplateAliasInput, optFns ...func(*quicksight.Options)) (*quicksight.CreateTemplateAliasOutput, error)
	UpdateTemplateAlias(ctx context.Context, params *quicksight.UpdateTemplateAliasInput, optFns ...func(*quicksight.Options)) (*quicksight.UpdateTemplateAliasOutput, error)
	DescribeTemplateAlias(ctx context.Context, params *quicksight.DescribeTemplateAliasInput, optFns ...func(*quicksight.Options)) (*quicksight.DescribeTemplateAliasOutput, error)
}

// timeout is how long UCP waits for an operation. AWS answers each call at once, and UCP only waits for its reads to
// show the change.
const timeout = 10 * time.Minute

var inRegion = awsapi.InRegion[quicksight.Options]

var (
	// accountPattern is the documented pattern of AwsAccountId.
	accountPattern = regexp.MustCompile(`^[0-9]{12}$`)

	// idPattern is the documented pattern of a folder, member, or template ID, and of an alias name that isn't
	// reserved.
	idPattern = regexp.MustCompile(`^[\w\-]+$`)
)

// invalid returns the client failure of a template AWS would reject, before any AWS call.
func invalid(typeName string, format string, args ...any) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s: %s", typeName, fmt.Sprintf(format, args...)),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// busy returns the failure of a change AWS rejected with ConflictException because it is still changing what the
// change belongs to, what. UCP answers 409 and sends nothing more, so the caller deploys again later.
func busy(typeName string, what string, cause error) error {
	return &native.Failure{
		Message: fmt.Sprintf("%s: AWS is still changing the %s, so nothing was changed. Deploy again once it is done, and give "+
			"resources of the same %s dependsOn on each other: %s", typeName, what, what, cause.Error()),
		ClientError: true,
		Conflict:    true,
		Outcome:     native.OutcomeRejected,
	}
}

// startError returns the error of a call that starts a change: a conflict for ConflictException, and AWS's error
// otherwise, as a rejected client failure when AWS answered with a 4xx status.
func startError(typeName string, what string, err error) error {
	if awsapi.HasCode(err, "ConflictException") {
		return busy(typeName, what, err)
	}
	return awsapi.Rejected(err)
}

// stringProperty returns the property name of properties, or "" when it is absent. It returns a client failure when
// the value isn't a string that check accepts.
func stringProperty(typeName string, properties map[string]any, name string, check func(string) bool, rule string) (string, error) {
	value, present := properties[name]
	if !present || value == nil {
		return "", nil
	}
	s, ok := value.(string)
	if !ok || !check(s) {
		return "", invalid(typeName, "%s must be %s", name, rule)
	}
	return s, nil
}

// accountID returns the template's AwsAccountId, or "" when it is absent.
func accountID(typeName string, properties map[string]any) (string, error) {
	return stringProperty(typeName, properties, "AwsAccountId", accountPattern.MatchString, "a 12-digit AWS account ID")
}

// resourceID returns the property name, the ID of a Quick Sight resource of at most max characters, or "" when it is
// absent.
func resourceID(typeName string, properties map[string]any, name string, max int) (string, error) {
	return stringProperty(typeName, properties, name, func(s string) bool { return validID(s, max) },
		fmt.Sprintf("1 to %d letters, digits, underscores, and hyphens", max))
}

// validID reports whether s is 1 to max characters matching idPattern.
func validID(s string, max int) bool {
	return len(s) <= max && idPattern.MatchString(s)
}
