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
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// DeploymentResourceTypeLabel holds the Radius resource type, with "/" replaced by "." because
	// label values cannot contain "/". Tools such as Argo CD can show it (resource.customLabels).
	DeploymentResourceTypeLabel = "radapp.io/resource-type"

	// DeploymentResourcePhraseLabel mirrors status.phrase so tools that only read labels can show it.
	DeploymentResourcePhraseLabel = "radapp.io/phrase"
)

// syncDisplayLabels sets the resource-type and phrase labels on a DeploymentResource.
// It patches only when a value changed, so repeated reconciles do not cause extra writes.
func syncDisplayLabels(ctx context.Context, c client.Client, deploymentResource *radappiov1alpha3.DeploymentResource) error {
	desired := map[string]string{
		DeploymentResourcePhraseLabel: string(deploymentResource.Status.Phrase),
	}
	if id, err := resources.ParseResource(deploymentResource.Spec.Id); err == nil {
		if t := strings.ReplaceAll(id.Type(), "/", "."); len(validation.IsValidLabelValue(t)) == 0 {
			desired[DeploymentResourceTypeLabel] = t
		}
	}

	changed := false
	for k, v := range desired {
		if deploymentResource.Labels[k] != v {
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}

	base := deploymentResource.DeepCopy()
	if deploymentResource.Labels == nil {
		deploymentResource.Labels = map[string]string{}
	}
	for k, v := range desired {
		deploymentResource.Labels[k] = v
	}
	return c.Patch(ctx, deploymentResource, client.MergeFrom(base))
}
