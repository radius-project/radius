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

// MsgSkippingResource is logged for each resource the cascade cannot delete, so the count shown in
// the confirmation prompt cannot quietly disagree with what was actually deleted.
const MsgSkippingResource = "  Warning: skipping %s because its resource ID or type is missing. It must be deleted manually."

// maxParallelDeletes bounds the number of deletions in flight. Each delete holds a long-running
// operation poller open against the RP, and an environment cascade can span every resource in
// every application, so the fan-out is capped to avoid overwhelming the server.
const maxParallelDeletes = 10

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
// have already been deleted. The ID of each resource is logged before its deletion is started,
// because output.Interface implementations are not guaranteed to be thread-safe and logging up
// front keeps the output deterministic.
//
// A resource missing an ID or type cannot be addressed and is skipped with a warning rather than
// silently dropped, so the caller's reported count cannot disagree with what was deleted.
//
// Deletions are limited to maxParallelDeletes at a time. On the first failure errgroup cancels the
// shared context, which abandons every other delete. Those deletes are left in mixed states: some
// were already accepted by the server and are still running there, some were canceled before the
// request was sent, and some queued behind the concurrency limit may never have started. The
// command reports a single error, so the outcome of the rest is unknown. Re-running the command is
// the way to converge, which is safe because deleting an already-deleted resource is treated as
// success.
func DeleteResourcesInParallel(ctx context.Context, client clients.ApplicationsManagementClient, out output.Interface, resources []generated.GenericResource, force bool) error {
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelDeletes)

	for _, resource := range resources {
		if resource.ID == nil || resource.Type == nil {
			out.LogInfo(MsgSkippingResource, describeResource(resource))
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

// describeResource returns the most identifying label available for a resource, for use in
// messages about resources that cannot be deleted.
func describeResource(resource generated.GenericResource) string {
	switch {
	case resource.ID != nil:
		return *resource.ID
	case resource.Name != nil:
		return *resource.Name
	default:
		return "an unnamed resource"
	}
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
