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

package clients

import "strings"

// AWSResourceOwnerHeader carries the owner of an AWS resource on a PUT or DELETE to UCP's AWS plane. UCP stores the
// owner of each alias and rejects a change from a different owner, so two applications that reuse an alias in one
// account and region cannot change each other's resources.
const AWSResourceOwnerHeader = "X-Radius-Resource-Owner"

// AWSResourceIDHeader carries the AWS resource ID a POST /:get reads, such as the ID of a recipe's output resource.
// UCP reads AWS by the identifier in the ID's last segment, decoded the way UCP encoded it in a response. Without the
// header, the body's top-level name is the AWS identifier itself and is never decoded. It is a header rather than a
// body field so a template, whose existing block becomes the body, cannot set it.
const AWSResourceIDHeader = "X-Radius-AWS-Resource-ID"

// AWSResourceNoRecordCode is the error code of the 404 UCP returns for a GET of an AWS resource ID that has no record,
// such as a resource Radius did not deploy. A 404 for a record whose AWS resource no longer exists has the code
// NotFound instead.
const AWSResourceNoRecordCode = "AWSResourceNoRecord"

// AWSRecipeOwner returns the owner of the AWS resources a recipe deploys:
// "recipe;<application ID>;<environment ID>;<Radius resource ID>". UCP compares owners ignoring case, because Radius
// IDs are not case-sensitive.
func AWSRecipeOwner(applicationID string, environmentID string, resourceID string) string {
	return strings.Join([]string{"recipe", applicationID, environmentID, resourceID}, ";")
}

// AWSDeploymentOwner returns the owner of the AWS resources in a template deployed with rad deploy:
// "deploy;<resource group ID>;<application ID>;<environment ID>", with an empty application ID when there is no
// application. resourceGroup is the name of the Radius resource group.
func AWSDeploymentOwner(resourceGroup string, applicationID string, environmentID string) string {
	return strings.Join([]string{"deploy", constructRadiusDeploymentScope(resourceGroup), applicationID, environmentID}, ";")
}
