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
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	schemautil "github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/resources"
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

const (
	managedSecretResourceType  = "Radius.Security/secrets"
	managedSecretDeleteTimeout = 5 * time.Minute
	managedSecretPollInterval  = time.Second
)

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
// A managed secret selected together with its producer is deleted by the server, not separately
// by the CLI. After deleting the producers, this helper verifies that those secrets disappear
// before returning, so callers can safely remove the application or environment.
// If a producer is already absent, an inactive remaining secret is deleted directly.
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
func DeleteResourcesInParallel(ctx context.Context, client clients.ApplicationsManagementClient, out output.Interface, selected []generated.GenericResource, force bool) error {
	managedSecrets, err := selectedManagedSecrets(selected)
	if err != nil {
		return err
	}

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelDeletes)
	missing := make([]bool, len(selected))

	for i, resource := range selected {
		if resource.ID == nil || resource.Type == nil {
			out.LogInfo(MsgSkippingResource, describeResource(resource))
			continue
		}

		if owner, managed := managedSecrets[deleteResourceKey(*resource.ID)]; managed {
			out.LogInfo("  Waiting for %s to delete its managed secret %s...", owner, *resource.ID)
			continue
		}

		out.LogInfo(MsgDeletingResource, *resource.ID)

		resourceType := *resource.Type
		resourceID := *resource.ID
		g.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			deleted, err := client.DeleteResource(groupCtx, resourceType, resourceID, force)
			if err != nil && !clients.Is404Error(err) {
				return err
			}
			missing[i] = !deleted || clients.Is404Error(err)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	if len(managedSecrets) == 0 {
		return ctx.Err()
	}
	missingOwners := map[string]bool{}
	for i, resource := range selected {
		if missing[i] {
			missingOwners[deleteResourceKey(*resource.ID)] = true
		}
	}

	// Wait cancels the first group's context even on success. Start from the caller's context.
	waitCtx, cancel := context.WithTimeout(ctx, managedSecretDeleteTimeout)
	defer cancel()
	g, groupCtx = errgroup.WithContext(waitCtx)
	g.SetLimit(maxParallelDeletes)
	for id, owner := range managedSecrets {
		deleteOrphan := missingOwners[deleteResourceKey(owner)]
		if deleteOrphan {
			out.LogInfo("  Resource %s is already absent; cleaning up its remaining managed secret %s...", owner, id)
		}
		g.Go(func() error {
			return waitForManagedSecretDeletion(groupCtx, client, id, owner, deleteOrphan, force)
		})
	}
	return g.Wait()
}

func deleteResourceKey(id string) string {
	return strings.ToLower(strings.TrimSuffix(id, "/"))
}

// selectedManagedSecrets maps selected secret IDs to selected producer IDs. The public name is
// resolved in the producer's root scope, not the workspace scope or a guessed naming convention.
func selectedManagedSecrets(selected []generated.GenericResource) (map[string]string, error) {
	addressable := map[string]generated.GenericResource{}
	for _, resource := range selected {
		if resource.ID != nil && resource.Type != nil {
			addressable[deleteResourceKey(*resource.ID)] = resource
		}
	}

	owners := map[string]string{}
	for _, resource := range selected {
		if resource.ID == nil || resource.Type == nil {
			continue
		}
		raw, exists := resource.Properties[schemautil.SecretsBlockPropertyName]
		if !exists || raw == nil {
			continue
		}
		secrets, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("resource %q has invalid properties.secrets: expected an object", *resource.ID)
		}
		rawName, exists := secrets[schemautil.SecretNameReferenceKey]
		if !exists {
			continue
		}
		name, ok := rawName.(string)
		if !ok || len(validation.IsDNS1123Label(strings.ToLower(name))) != 0 {
			return nil, fmt.Errorf("resource %q has invalid properties.secrets.name: expected a secret resource name", *resource.ID)
		}
		ownerID, err := resources.ParseResource(deleteResourceKey(*resource.ID))
		if err != nil {
			return nil, fmt.Errorf("cannot resolve managed secret for resource %q: %w", *resource.ID, err)
		}
		secretID := ownerID.RootScope() + "/providers/" + managedSecretResourceType + "/" + name
		key := deleteResourceKey(secretID)
		child, selected := addressable[key]
		if !selected {
			continue
		}
		if !strings.EqualFold(*child.Type, managedSecretResourceType) {
			return nil, fmt.Errorf("managed secret %q has unexpected resource type %q", *child.ID, *child.Type)
		}
		if other, exists := owners[key]; exists && !strings.EqualFold(other, *resource.ID) {
			return nil, fmt.Errorf("managed secret %q is claimed by both %q and %q", *child.ID, other, *resource.ID)
		}
		owners[key] = *resource.ID
	}

	for child := range owners {
		seen := map[string]bool{}
		for id := child; owners[id] != ""; id = deleteResourceKey(owners[id]) {
			if seen[id] {
				return nil, fmt.Errorf("managed secret ownership cycle involving %q", child)
			}
			seen[id] = true
		}
	}
	return owners, nil
}

func waitForManagedSecretDeletion(ctx context.Context, client clients.ApplicationsManagementClient, id, owner string, deleteOrphan, force bool) error {
	lastState := "unknown"
	err := wait.PollUntilContextCancel(ctx, managedSecretPollInterval, true, func(ctx context.Context) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		secret, err := client.GetResource(ctx, managedSecretResourceType, id)
		if clients.Is404Error(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		lastState = "unknown"
		if raw, exists := secret.Properties["provisioningState"]; exists && raw != nil {
			state, ok := raw.(string)
			if !ok {
				return false, fmt.Errorf("invalid properties.provisioningState: expected a string")
			}
			lastState = state
		}
		failed := strings.EqualFold(lastState, string(v1.ProvisioningStateFailed)) || strings.EqualFold(lastState, string(v1.ProvisioningStateCanceled))
		// A missing producer may still have an earlier cascade in flight. Only submit a new
		// DELETE once the secret is terminal, even when the caller requested force.
		if deleteOrphan && (lastState == "" || strings.EqualFold(lastState, string(v1.ProvisioningStateSucceeded)) || failed) {
			_, err := client.DeleteResource(ctx, managedSecretResourceType, id, force)
			if err != nil && !clients.Is404Error(err) {
				return false, err
			}
			deleteOrphan = false
			return false, nil
		}
		if failed {
			return false, fmt.Errorf("managed secret cleanup reached state %q", lastState)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("managed secret %q for resource %q was not confirmed deleted (last state %q); application/environment deletion stopped, inspect the secret before retrying: %w", id, owner, lastState, err)
	}
	return nil
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
