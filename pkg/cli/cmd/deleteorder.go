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
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/recipepack"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// DeleteTier identifies a stage in an ordered delete. Lower tiers are deleted first, so a resource
// is always deleted before the resources it depends on.
type DeleteTier int

const (
	// DeleteTierWorkload covers ordinary workloads such as containers, gateways, extenders and
	// user-defined types. These are recipe-driven and depend on everything in the later tiers.
	DeleteTierWorkload DeleteTier = iota

	// DeleteTierSecuritySecret covers Radius.Security/secrets. A secret may itself be recipe-driven,
	// so it cannot be deleted alongside the workloads in the first tier. It may also supply
	// credentials to a settings resource in the final tier, so it cannot be deleted with those
	// either. It therefore needs a tier of its own between the two.
	//
	// Known limitation: secrets within this tier are deleted concurrently, and a type cannot be
	// ordered against itself. This only matters when a secret holds credentials that Radius has to
	// resolve in order to run another secret's delete recipe: deleting a resource runs its recipe,
	// and engine deleteCore resolves the credentials named by the environment's Bicep
	// authentication or Terraform settings before invoking the driver. If that credential secret is
	// deleted first, the recipe can no longer run and the remaining secret cannot be deleted.
	//
	// The secret's own data is never read for this, so ordering is irrelevant whenever the
	// environment configures no credentials at all. A public recipe registry is not on its own
	// enough to guarantee that: the Bicep driver gathers every secret named by
	// RecipeConfig.Bicep.Authentication, and the Terraform driver additionally resolves the secrets
	// backing its provider configuration and environment variables. Closing the gap entirely needs
	// ordering derived from the references between individual resources rather than their types.
	DeleteTierSecuritySecret

	// DeleteTierSecretStore covers the legacy Applications.Core/secretStores, which plays the same
	// credential-supplying role as a Radius.Security/secrets but is resolved through a separate
	// code path. It is deleted after Radius.Security/secrets so that a secret store backing a
	// secret is still resolvable while that secret is being deleted.
	DeleteTierSecretStore

	// DeleteTierApplication covers applications in both the Applications.Core and Radius.Core
	// namespaces. Applications own the workloads deleted in the earlier tiers.
	DeleteTierApplication

	// DeleteTierEnvironment covers environments in both namespaces along with the configuration
	// they reference: recipe packs, Terraform settings and Bicep settings. Every recipe-driven
	// delete in an earlier tier has to load this configuration in order to run, so it must survive
	// until all of those deletes have finished.
	DeleteTierEnvironment

	// numDeleteTiers is the number of tiers. It must be updated when a tier is added.
	numDeleteTiers = int(DeleteTierEnvironment) + 1
)

// securitySecretsResourceType is declared locally because the repository has no single canonical
// constant for it: the type name is repeated in several packages, each with its own unexported
// copy. Every other type in the tier table below uses the shared constant from pkg/corerp/datamodel
// so that the table cannot silently drift from the datamodel's definition -- a stale literal here
// would stop matching and fall through to DeleteTierWorkload, reinstating the ordering bug with no
// compile error.
const securitySecretsResourceType = "Radius.Security/secrets"

// deleteTiersByResourceType maps a resource type to the tier it is deleted in. Types that are
// absent are deleted in DeleteTierWorkload, which is the correct default: an unrecognized type is
// assumed to be a recipe-driven workload that depends on the configuration in the later tiers.
//
// The order encoded here is derived from what a recipe-driven delete loads while it runs. Deleting
// a resource runs its recipe's delete, which loads the environment, the environment's recipe pack
// and Terraform or Bicep settings, and any secrets those settings reference. Deleting any of those
// first is what leaves the recipe unable to run, which is the failure this ordering prevents.
//
// Keys are matched case-insensitively through deleteTierLookup, so they are written here in their
// canonical casing.
//
// This table orders the deletes the CLI issues itself. It does not constrain deletes the backend
// cascades on its own: deleting a dynamic resource deletes the managed Radius.Security/secrets
// resource it owns, and the materializer issues that delete without polling it to completion. Such
// a cascade can therefore still be running when this tier table reaches the secret tier, so the
// table is an ordering of resource types rather than a complete dependency graph.
var deleteTiersByResourceType = map[string]DeleteTier{
	// Credential sources, innermost first.
	securitySecretsResourceType:       DeleteTierSecuritySecret,
	datamodel.SecretStoreResourceType: DeleteTierSecretStore,

	// Applications own the workloads in DeleteTierWorkload.
	datamodel.ApplicationResourceType:                  DeleteTierApplication,
	datamodel.ApplicationResourceType_v20250801preview: DeleteTierApplication,

	// Environments and the configuration a recipe delete resolves through them.
	datamodel.EnvironmentResourceType:                  DeleteTierEnvironment,
	datamodel.EnvironmentResourceType_v20250801preview: DeleteTierEnvironment,
	recipepack.ResourceType:                            DeleteTierEnvironment,
	datamodel.TerraformSettingsResourceType:            DeleteTierEnvironment,
	datamodel.BicepSettingsResourceType:                DeleteTierEnvironment,
}

// deleteTierLookup is deleteTiersByResourceType keyed by lowercased resource type, so that lookups
// are case-insensitive. Resource types are returned by the API in their declared casing, which is
// not guaranteed to match the casing used above.
var deleteTierLookup = func() map[string]DeleteTier {
	lookup := make(map[string]DeleteTier, len(deleteTiersByResourceType))
	for resourceType, tier := range deleteTiersByResourceType {
		lookup[strings.ToLower(resourceType)] = tier
	}

	return lookup
}()

// DeleteTierForResourceType returns the tier a resource type is deleted in, matching the type
// case-insensitively. An unrecognized type is treated as a workload.
func DeleteTierForResourceType(resourceType string) DeleteTier {
	if tier, ok := deleteTierLookup[strings.ToLower(resourceType)]; ok {
		return tier
	}

	return DeleteTierWorkload
}

// GroupResourcesByDeleteTier partitions resources into the tiers they must be deleted in, ordered
// from the first tier to delete to the last. The returned slice always has one entry per tier, so
// callers can iterate it directly; tiers with no resources are empty.
//
// A resource missing a type cannot be classified and is placed in the first tier. Such a resource
// cannot be addressed for deletion at all, and is reported by the delete itself rather than dropped
// here, so that the set shown to the user stays consistent with the set that was acted on.
func GroupResourcesByDeleteTier(resources []generated.GenericResource) [][]generated.GenericResource {
	tiers := make([][]generated.GenericResource, numDeleteTiers)

	for _, resource := range resources {
		tier := DeleteTierWorkload
		if resource.Type != nil {
			tier = DeleteTierForResourceType(*resource.Type)
		}

		tiers[tier] = append(tiers[tier], resource)
	}

	return tiers
}

// DeleteOrderedResourceTypes returns every resource type with an explicit tier, ordered by tier and
// then alphabetically. It exists so that tests and documentation can describe the ordering without
// duplicating the table.
func DeleteOrderedResourceTypes() []string {
	types := make([]string, 0, len(deleteTiersByResourceType))
	for resourceType := range deleteTiersByResourceType {
		types = append(types, resourceType)
	}

	sort.Slice(types, func(i, j int) bool {
		if deleteTiersByResourceType[types[i]] != deleteTiersByResourceType[types[j]] {
			return deleteTiersByResourceType[types[i]] < deleteTiersByResourceType[types[j]]
		}

		return types[i] < types[j]
	})

	return types
}

// DeleteResourcesInTiers deletes resources in dependency order, one tier at a time. A tier is only
// started once every delete in the previous tier has finished, so that a recipe running in an
// earlier tier can still resolve the environment, settings and secrets it needs.
//
// Within a tier, deletes run concurrently up to maxParallelDeletes because nothing in a tier
// depends on anything else in it.
//
// Unlike DeleteResourcesInParallel, a failure does not abandon the deletes running alongside it.
// Every delete in the tier is allowed to finish and all of their errors are reported together. A
// destructive command that stops partway through leaves the user unable to tell which resources
// were deleted, and re-running it is the only way to converge; reporting the full set of failures
// makes that outcome legible. Deleting an already-deleted resource succeeds, so re-running is safe.
//
// Later tiers are skipped once a tier has failed, because the resources that failed to delete may
// still depend on them.
func DeleteResourcesInTiers(ctx context.Context, client clients.ApplicationsManagementClient, out output.Interface, tiers [][]generated.GenericResource, force bool) error {
	for _, tier := range tiers {
		if len(tier) == 0 {
			continue
		}

		if err := deleteResourceTier(ctx, client, out, tier, force); err != nil {
			return err
		}
	}

	return nil
}

// deleteResourceTier deletes one tier concurrently, waiting for every delete to finish and joining
// their errors. Resources are logged before the deletes start because output.Interface
// implementations are not guaranteed to be thread-safe.
func deleteResourceTier(ctx context.Context, client clients.ApplicationsManagementClient, out output.Interface, resources []generated.GenericResource, force bool) error {
	deletable := make([]generated.GenericResource, 0, len(resources))

	var unaddressable []error
	for _, resource := range resources {
		// A resource with no ID or type cannot be addressed, so it cannot be deleted. This is
		// reported as an error rather than a warning: the caller deletes the resource group once
		// the tiers succeed, and a group deleted while one of its resources survives orphans that
		// resource with nothing left pointing at it.
		if resource.ID == nil || resource.Type == nil {
			out.LogInfo(MsgSkippingResource, describeResource(resource))
			unaddressable = append(unaddressable, fmt.Errorf("cannot delete %s: its resource ID or type is missing", describeResource(resource)))

			continue
		}

		out.LogInfo(MsgDeletingResource, *resource.ID)
		deletable = append(deletable, resource)
	}

	errs := make([]error, len(deletable))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxParallelDeletes)

	for i, resource := range deletable {
		// The slot is taken before the goroutine is started, not inside it, so that a large tier
		// does not allocate one goroutine per resource while only the API calls are bounded. This
		// loop blocks here until a slot frees up, which caps goroutines and in-flight deletes
		// together.
		semaphore <- struct{}{}

		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()

			// ctx is deliberately not derived from an errgroup: one failure must not cancel the
			// deletes running alongside it.
			_, err := client.DeleteResource(ctx, *resource.Type, *resource.ID, force)
			if err != nil && !clients.Is404Error(err) {
				errs[i] = fmt.Errorf("failed to delete %s: %w", *resource.ID, err)
			}
		}()
	}

	wg.Wait()

	return errors.Join(append(errs, unaddressable...)...)
}

// maxDescribedResources bounds how many resources DescribeResources names individually. An error
// message that lists every resource in a large group is unreadable in a terminal, and the names
// beyond the first few add nothing: the user re-runs the command rather than acting on each one.
const maxDescribedResources = 10

// DescribeResources renders resources as a human-readable list for use in error messages, naming
// at most maxDescribedResources of them and summarizing the rest as a count. It returns the empty
// string when there are no resources, so callers must only use it when there is something to
// describe.
func DescribeResources(resources []generated.GenericResource) string {
	if len(resources) == 0 {
		return ""
	}

	named := resources
	if len(named) > maxDescribedResources {
		named = named[:maxDescribedResources]
	}

	descriptions := make([]string, 0, len(named))
	for _, resource := range named {
		descriptions = append(descriptions, describeResource(resource))
	}

	list := strings.Join(descriptions, ", ")
	if remaining := len(resources) - len(named); remaining > 0 {
		list = fmt.Sprintf("%s and %d more", list, remaining)
	}

	if len(resources) == 1 {
		return fmt.Sprintf("1 resource: %s", list)
	}

	return fmt.Sprintf("%d resources: %s", len(resources), list)
}
