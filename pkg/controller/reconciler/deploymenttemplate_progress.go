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

package reconciler

import (
	"context"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

// DeploymentResourceDeployingAnnotation is set on every DeploymentResource owned by a DeploymentTemplate
// while Radius deploys that template. The value is the template generation being deployed. A Radius
// deployment re-applies every resource in the template, so each owned resource is part of the rollout.
const DeploymentResourceDeployingAnnotation = "radapp.io/deploying-generation"

// markOwnedResourcesDeploying annotates the DeploymentResources owned by the template with the
// generation being deployed.
func markOwnedResourcesDeploying(ctx context.Context, c client.Client, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) error {
	return setOwnedResourcesDeploying(ctx, c, deploymentTemplate, strconv.FormatInt(deploymentTemplate.Generation, 10))
}

// clearOwnedResourcesDeploying removes the deploying annotation from the DeploymentResources owned
// by the template.
func clearOwnedResourcesDeploying(ctx context.Context, c client.Client, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) error {
	return setOwnedResourcesDeploying(ctx, c, deploymentTemplate, "")
}

// setOwnedResourcesDeploying sets the deploying annotation to value, or removes it when value is empty.
// Resources that are being deleted are skipped.
func setOwnedResourcesDeploying(ctx context.Context, c client.Client, deploymentTemplate *radappiov1alpha3.DeploymentTemplate, value string) error {
	list := &radappiov1alpha3.DeploymentResourceList{}
	if err := c.List(ctx, list, client.InNamespace(deploymentTemplate.Namespace)); err != nil {
		return err
	}

	for i := range list.Items {
		dr := &list.Items[i]
		if !isOwnedBy(*dr, deploymentTemplate) || dr.DeletionTimestamp != nil {
			continue
		}
		if dr.Annotations[DeploymentResourceDeployingAnnotation] == value {
			continue
		}

		base := dr.DeepCopy()
		if value == "" {
			delete(dr.Annotations, DeploymentResourceDeployingAnnotation)
		} else {
			if dr.Annotations == nil {
				dr.Annotations = map[string]string{}
			}
			dr.Annotations[DeploymentResourceDeployingAnnotation] = value
		}
		if err := c.Patch(ctx, dr, client.MergeFrom(base)); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}
