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
