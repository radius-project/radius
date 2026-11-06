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
	"fmt"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clients"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// untrackedNamespace is the resource namespace whose output resources get no DeploymentResource.
// Applications, environments and recipe packs are containers for other resources: the Argo CD
// Application and DeploymentTemplate already stand for the application, so a tile for it is noise.
const untrackedNamespace = "Radius.Core"

// isTrackedOutputResource reports whether an output resource gets its own DeploymentResource.
func isTrackedOutputResource(resourceID string) bool {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return true
	}
	namespace, _, _ := strings.Cut(id.Type(), "/")
	return !strings.EqualFold(namespace, untrackedNamespace)
}

// untrackedDeleteOrder returns the untracked output resources in the order they must be deleted:
// applications before the environments they run in, and environments before their recipe packs.
func untrackedDeleteOrder(outputResources []string) []string {
	rank := func(resourceID string) int {
		id, err := resources.ParseResource(resourceID)
		if err != nil {
			return 3
		}
		typ := id.Type()
		switch strings.ToLower(typ[strings.LastIndex(typ, "/")+1:]) {
		case "applications":
			return 0
		case "environments":
			return 1
		case "recipepacks":
			return 2
		}
		return 3
	}

	var untracked []string
	for _, resourceID := range outputResources {
		if !isTrackedOutputResource(resourceID) {
			untracked = append(untracked, resourceID)
		}
	}
	sort.SliceStable(untracked, func(i, j int) bool { return rank(untracked[i]) < rank(untracked[j]) })
	return untracked
}

// deleteUntrackedOutputResources deletes the output resources that have no DeploymentResource.
// It runs after every owned DeploymentResource is gone, so nothing in the template still uses them.
func (r *DeploymentTemplateReconciler) deleteUntrackedOutputResources(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	for _, resourceID := range untrackedDeleteOrder(deploymentTemplate.Status.OutputResources) {
		logger.Info("Deleting untracked output resource.", "resourceId", resourceID)
		poller, err := r.ResourceDeploymentsClient.Delete(ctx, resourceID, deleteAPIVersion(deploymentTemplate.Spec.Template, resourceID))
		if err != nil {
			if clients.Is404Error(err) {
				continue
			}
			return fmt.Errorf("failed to delete %s: %w", resourceID, err)
		}
		if poller == nil {
			continue
		}
		if _, err := poller.PollUntilDone(ctx, nil); err != nil && !clients.Is404Error(err) {
			return fmt.Errorf("failed to delete %s: %w", resourceID, err)
		}
	}
	return nil
}
