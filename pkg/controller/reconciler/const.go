/*
Copyright 2023.

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

package reconciler

import "time"

const (
	// PollingDelay is the amount of time to wait between polling for the status of a resource.
	PollingDelay time.Duration = 5 * time.Second

	// DeleteRetryDelay is the default RequeueAfter used when a DeploymentResource
	// delete operation returns a transient error. Using a fixed bound instead of
	// returning the error avoids controller-runtime's exponential rate-limiter,
	// which climbs to ~16m and extends total drain time.
	DeleteRetryDelay time.Duration = 30 * time.Second

	// EventDeploymentResourceDeleteSkipped is emitted when the controller skips deleting the
	// resource referenced by a DeploymentResource because the object is not a controller-owned
	// child of a DeploymentTemplate whose deployment scope covers Spec.Id.
	EventDeploymentResourceDeleteSkipped = "DeploymentResourceDeleteSkipped"

	// deploymentTemplateKind is the Kind of the DeploymentTemplate CRD, used when checking the
	// controller owner reference on a DeploymentResource.
	deploymentTemplateKind = "DeploymentTemplate"

	// DeploymentTemplateFinalizer is the name of the finalizer added to DeploymentTemplates.
	DeploymentTemplateFinalizer = "radapp.io/deployment-template-finalizer"

	// DeploymentResourceFinalizer is the name of the finalizer added to DeploymentResources.
	DeploymentResourceFinalizer = "radapp.io/deployment-resource-finalizer"

	// GitRepositoryHttpRetryCount is the number of times to retry GitRepository HTTP requests.
	GitRepositoryHttpRetryCount = 9
)
