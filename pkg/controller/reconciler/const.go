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

	// AnnotationRadiusEnabled is the name of the annotation that indicates if a Deployment has Radius enabled.
	AnnotationRadiusEnabled = "radapp.io/enabled"

	// AnnotationRadiusConnectionPrefix is the name of the annotation that indicates the name of the connection to use.
	AnnotationRadiusConnectionPrefix = "radapp.io/connection-"

	// AnnotationRadiusStatus is the name of the annotation that indicates the status of a Deployment.
	AnnotationRadiusStatus = "radapp.io/status"

	// AnnotationRadiusConfigurationHash is the name of the annotation that indicates the hash of the configuration.
	AnnotationRadiusConfigurationHash = "radapp.io/configuration-hash"

	// AnnotationRadiusEnvironment is the name of the annotation that indicates the name of the environment. If unset,
	// the value 'default' will be used as the environment name.
	AnnotationRadiusEnvironment = "radapp.io/environment"

	// AnnotationRadiusApplication is the name of the annotation that indicates the name of the application. If unset,
	// the namespace of the Deployment will be used as the application name.
	AnnotationRadiusApplication = "radapp.io/application"

	// DeploymentFinalizer is the name of the finalizer added to Deployments.
	DeploymentFinalizer = "radapp.io/deployment-finalizer"

	// RecipeFinalizer is the name of the finalizer added to Recipes.
	RecipeFinalizer = "radapp.io/recipe-finalizer"

	// DeploymentTemplateFinalizer is the name of the finalizer added to DeploymentTemplates.
	DeploymentTemplateFinalizer = "radapp.io/deployment-template-finalizer"

	// DeploymentResourceFinalizer is the name of the finalizer added to DeploymentResources.
	DeploymentResourceFinalizer = "radapp.io/deployment-resource-finalizer"

	// RadiusSystemNamespace is the name of the system namespace where Radius resources are stored.
	RadiusSystemNamespace = "radius-system"

	// GitRepositoryHttpRetryCount is the number of times to retry GitRepository HTTP requests.
	GitRepositoryHttpRetryCount = 9
)
