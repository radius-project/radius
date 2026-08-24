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

package cmd

import (
	"context"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// MsgDeletingResource is logged for each resource before its deletion is started.
const MsgDeletingResource = "  Deleting %s..."

// PreviewResourceID builds a fully qualified Radius.Core resource ID from a workspace
// scope, resource type and resource name.
func PreviewResourceID(scope string, resourceType string, name string) string {
	return scope + "/providers/" + resourceType + "/" + name
}

// PreviewApplicationID builds a fully qualified Radius.Core application ID.
func PreviewApplicationID(scope string, applicationName string) string {
	return PreviewResourceID(scope, datamodel.ApplicationResourceType_v20250801preview, applicationName)
}

// PreviewEnvironmentID builds a fully qualified Radius.Core environment ID.
func PreviewEnvironmentID(scope string, environmentName string) string {
	return PreviewResourceID(scope, datamodel.EnvironmentResourceType_v20250801preview, environmentName)
}

// DeleteResourcesInParallel deletes the given resources concurrently, tolerating resources that
// have already been deleted. Resources missing an ID or type are skipped. The name of each
// resource is logged before its deletion is started, because output.Interface implementations are
// not guaranteed to be thread-safe and logging up front keeps the output deterministic.
func DeleteResourcesInParallel(ctx context.Context, client clients.ApplicationsManagementClient, out output.Interface, resources []generated.GenericResource, force bool) error {
	g, groupCtx := errgroup.WithContext(ctx)
	for _, resource := range resources {
		if resource.ID == nil || resource.Type == nil {
			continue
		}

		out.LogInfo(MsgDeletingResource, *resource.ID)

		resourceType := *resource.Type
		resourceID := *resource.ID
		g.Go(func() error {
			_, err := client.DeleteResource(groupCtx, resourceType, resourceID, force)
			if err != nil && !clients.Is404Error(err) {
				return err
			}
			return nil
		})
	}

	return g.Wait()
}

// ListPreviewApplicationsInEnvironment lists the Radius.Core applications in the workspace scope
// whose properties.environment references the given environment ID.
func ListPreviewApplicationsInEnvironment(ctx context.Context, client *corerpv20250801.ApplicationsClient, workspace *workspaces.Workspace, environmentID string) ([]corerpv20250801.ApplicationResource, error) {
	results := []corerpv20250801.ApplicationResource{}

	pager := client.NewListByScopePager(workspace.Scope, &corerpv20250801.ApplicationsClientListByScopeOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, application := range page.Value {
			if application == nil || application.Properties == nil || application.Properties.Environment == nil {
				continue
			}

			if strings.EqualFold(*application.Properties.Environment, environmentID) {
				results = append(results, *application)
			}
		}
	}

	return results, nil
}

// MergeResourcesByID concatenates the given resource lists, dropping duplicates by
// case-insensitive resource ID. Resources without an ID are dropped, since they cannot be deleted.
// When the same resource ID appears more than once, a representation carrying a type is preferred
// over one without, because a resource missing its type cannot be deleted.
func MergeResourcesByID(lists ...[]generated.GenericResource) []generated.GenericResource {
	indexes := map[string]int{}
	results := []generated.GenericResource{}

	for _, list := range lists {
		for _, resource := range list {
			if resource.ID == nil {
				continue
			}

			key := strings.ToLower(*resource.ID)
			if index, ok := indexes[key]; ok {
				if results[index].Type == nil && resource.Type != nil {
					results[index] = resource
				}
				continue
			}

			indexes[key] = len(results)
			results = append(results, resource)
		}
	}

	return results
}
