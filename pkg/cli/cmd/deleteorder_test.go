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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/to"
)

func TestDeleteTierForResourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceType string
		expected     DeleteTier
	}{
		{
			name:         "recipe-driven container is a workload",
			resourceType: "Radius.Compute/containers",
			expected:     DeleteTierWorkload,
		},
		{
			name:         "legacy extender is a workload",
			resourceType: "Applications.Core/extenders",
			expected:     DeleteTierWorkload,
		},
		{
			name:         "user-defined type is a workload",
			resourceType: "MyCompany.Widgets/widgets",
			expected:     DeleteTierWorkload,
		},
		{
			name:         "security secret has its own tier",
			resourceType: "Radius.Security/secrets",
			expected:     DeleteTierSecuritySecret,
		},
		{
			name:         "legacy secret store has its own tier",
			resourceType: "Applications.Core/secretStores",
			expected:     DeleteTierSecretStore,
		},
		{
			name:         "legacy application",
			resourceType: "Applications.Core/applications",
			expected:     DeleteTierApplication,
		},
		{
			name:         "preview application",
			resourceType: "Radius.Core/applications",
			expected:     DeleteTierApplication,
		},
		{
			name:         "legacy environment",
			resourceType: "Applications.Core/environments",
			expected:     DeleteTierEnvironment,
		},
		{
			name:         "preview environment",
			resourceType: "Radius.Core/environments",
			expected:     DeleteTierEnvironment,
		},
		{
			name:         "recipe pack is deleted with environments",
			resourceType: "Radius.Core/recipePacks",
			expected:     DeleteTierEnvironment,
		},
		{
			name:         "terraform settings are deleted with environments",
			resourceType: "Radius.Core/terraformSettings",
			expected:     DeleteTierEnvironment,
		},
		{
			name:         "bicep settings are deleted with environments",
			resourceType: "Radius.Core/bicepSettings",
			expected:     DeleteTierEnvironment,
		},
		{
			name:         "empty type is a workload",
			resourceType: "",
			expected:     DeleteTierWorkload,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, DeleteTierForResourceType(test.resourceType))
		})
	}
}

// TestDeleteTierForResourceType_CaseInsensitive asserts that classification does not depend on the
// casing the API happens to return, which is not guaranteed to match the table.
func TestDeleteTierForResourceType_CaseInsensitive(t *testing.T) {
	t.Parallel()

	for _, resourceType := range DeleteOrderedResourceTypes() {
		expected := DeleteTierForResourceType(resourceType)

		for _, variant := range []string{strings.ToLower(resourceType), strings.ToUpper(resourceType)} {
			require.Equal(t, expected, DeleteTierForResourceType(variant),
				"casing variant %q should classify the same as %q", variant, resourceType)
		}
	}
}

// TestDeleteOrderedResourceTypes_Ordering asserts the dependency order the fix relies on, stated in
// terms of the relationships rather than the tier numbers, so that inserting a tier does not
// require rewriting these expectations.
func TestDeleteOrderedResourceTypes_Ordering(t *testing.T) {
	t.Parallel()

	tier := DeleteTierForResourceType

	// Workloads are deleted before the credentials, applications and environments their recipes
	// resolve while they are being deleted. This is the ordering bug the fix exists to prevent.
	require.Less(t, tier("Radius.Compute/containers"), tier("Radius.Security/secrets"))
	require.Less(t, tier("Radius.Compute/containers"), tier("Radius.Core/applications"))
	require.Less(t, tier("Radius.Compute/containers"), tier("Radius.Core/environments"))
	require.Less(t, tier("Radius.Compute/containers"), tier("Radius.Core/recipePacks"))

	// A recipe-driven secret still needs its settings and environment, so it precedes them.
	require.Less(t, tier("Radius.Security/secrets"), tier("Radius.Core/terraformSettings"))
	require.Less(t, tier("Radius.Security/secrets"), tier("Radius.Core/environments"))

	// A secret store may back a secret, so it outlives it.
	require.Less(t, tier("Radius.Security/secrets"), tier("Applications.Core/secretStores"))

	// Applications outlive their workloads but not their environment.
	require.Less(t, tier("Radius.Core/applications"), tier("Radius.Core/environments"))

	// The recipe configuration is deleted in the same tier as the environment it hangs off, so no
	// recipe delete in an earlier tier can find it missing.
	require.Equal(t, tier("Radius.Core/environments"), tier("Radius.Core/recipePacks"))
	require.Equal(t, tier("Radius.Core/environments"), tier("Radius.Core/terraformSettings"))
	require.Equal(t, tier("Radius.Core/environments"), tier("Radius.Core/bicepSettings"))
}

func TestGroupResourcesByDeleteTier(t *testing.T) {
	t.Parallel()

	resource := func(resourceType string, name string) generated.GenericResource {
		return generated.GenericResource{
			ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/" + resourceType + "/" + name),
			Name: to.Ptr(name),
			Type: to.Ptr(resourceType),
		}
	}

	t.Run("partitions a full group into tiers", func(t *testing.T) {
		t.Parallel()

		resources := []generated.GenericResource{
			resource("Radius.Core/recipePacks", "pack"),
			resource("Radius.Compute/containers", "frontend"),
			resource("Radius.Core/environments", "env"),
			resource("Radius.Security/secrets", "secret"),
			resource("Radius.Core/applications", "app"),
			resource("Applications.Core/secretStores", "store"),
			resource("Radius.Compute/containers", "backend"),
		}

		tiers := GroupResourcesByDeleteTier(resources)
		require.Len(t, tiers, numDeleteTiers)

		require.Equal(t, []string{"frontend", "backend"}, resourceNames(tiers[DeleteTierWorkload]))
		require.Equal(t, []string{"secret"}, resourceNames(tiers[DeleteTierSecuritySecret]))
		require.Equal(t, []string{"store"}, resourceNames(tiers[DeleteTierSecretStore]))
		require.Equal(t, []string{"app"}, resourceNames(tiers[DeleteTierApplication]))
		require.Equal(t, []string{"pack", "env"}, resourceNames(tiers[DeleteTierEnvironment]))
	})

	t.Run("returns one empty tier per stage for no resources", func(t *testing.T) {
		t.Parallel()

		tiers := GroupResourcesByDeleteTier(nil)
		require.Len(t, tiers, numDeleteTiers)

		for tier, contents := range tiers {
			require.Empty(t, contents, "tier %d should be empty", tier)
		}
	})

	t.Run("keeps every resource", func(t *testing.T) {
		t.Parallel()

		resources := []generated.GenericResource{
			resource("Radius.Compute/containers", "frontend"),
			resource("Radius.Core/environments", "env"),
			resource("Radius.Core/applications", "app"),
		}

		tiers := GroupResourcesByDeleteTier(resources)

		total := 0
		for _, contents := range tiers {
			total += len(contents)
		}

		require.Equal(t, len(resources), total, "no resource may be dropped during classification")
	})

	// A resource with no type cannot be classified. It is kept rather than dropped so that the
	// delete can report it, and placed in the first tier so it cannot outlive its dependencies.
	t.Run("keeps a resource with no type in the first tier", func(t *testing.T) {
		t.Parallel()

		untyped := generated.GenericResource{
			ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Some.Type/typeless"),
			Name: to.Ptr("typeless"),
		}

		tiers := GroupResourcesByDeleteTier([]generated.GenericResource{untyped})

		require.Equal(t, []string{"typeless"}, resourceNames(tiers[DeleteTierWorkload]))
	})
}

func resourceNames(resources []generated.GenericResource) []string {
	names := make([]string, 0, len(resources))
	for _, resource := range resources {
		names = append(names, *resource.Name)
	}

	return names
}

// newTieredResources returns a resource per supplied type, named after its index, for tests that
// care about the tier a resource lands in rather than its identity.
func newTieredResources(resourceTypes ...string) []generated.GenericResource {
	resources := make([]generated.GenericResource, 0, len(resourceTypes))
	for i, resourceType := range resourceTypes {
		name := fmt.Sprintf("resource%d", i)
		resources = append(resources, generated.GenericResource{
			ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/" + resourceType + "/" + name),
			Name: to.Ptr(name),
			Type: to.Ptr(resourceType),
		})
	}

	return resources
}

// barrierProbe is how long the workload delete stays in flight while watching for an environment
// delete to start alongside it. An unordered implementation runs both concurrently and trips the
// probe immediately; a correctly ordered one cannot start the environment at all, so it waits out
// the full duration once.
const barrierProbe = 250 * time.Millisecond

// TestDeleteResourcesInTiers_WaitsForTierToComplete is the regression test for the ordering bug.
// It asserts completion ordering rather than request ordering: an environment delete must not start
// until every workload delete has returned, because a workload's recipe resolves the environment
// while it is being torn down.
func TestDeleteResourcesInTiers_WaitsForTierToComplete(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := clients.NewMockApplicationsManagementClient(ctrl)

	environmentStarted := make(chan struct{})

	// Recorded rather than asserted inside the callback: a failed assertion inside a mock callback
	// calls Goexit, which would abandon the delete and hang the wait below.
	var startedConcurrently atomic.Bool

	client.EXPECT().
		DeleteResource(gomock.Any(), "Radius.Compute/containers", gomock.Any(), false).
		DoAndReturn(func(ctx context.Context, resourceType string, resourceID string, force bool) (bool, error) {
			// Hold the workload delete open and watch for the environment delete starting beside
			// it, which is exactly the overlap that leaves the workload unable to tear itself down.
			select {
			case <-environmentStarted:
				startedConcurrently.Store(true)
			case <-time.After(barrierProbe):
			}

			return true, nil
		}).Times(1)

	client.EXPECT().
		DeleteResource(gomock.Any(), "Radius.Core/environments", gomock.Any(), false).
		DoAndReturn(func(ctx context.Context, resourceType string, resourceID string, force bool) (bool, error) {
			close(environmentStarted)

			return true, nil
		}).Times(1)

	// Listed environment-first so that a flat, unordered delete would submit the environment before
	// the workload. Ordering must come from the tiers, not from the enumeration order.
	resources := newTieredResources("Radius.Core/environments", "Radius.Compute/containers")
	tiers := GroupResourcesByDeleteTier(resources)

	err := DeleteResourcesInTiers(t.Context(), client, &output.MockOutput{}, tiers, false)
	require.NoError(t, err)

	require.False(t, startedConcurrently.Load(),
		"the environment delete started while a workload delete was still in flight")
}

// TestDeleteResourcesInTiers_CollectsErrors asserts that one failure does not abandon the deletes
// running alongside it, and that every failure is reported. A destructive command that stops
// partway through leaves the user unable to tell what was deleted.
func TestDeleteResourcesInTiers_CollectsErrors(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := clients.NewMockApplicationsManagementClient(ctrl)

	var attempted atomic.Int32

	client.EXPECT().
		DeleteResource(gomock.Any(), "Radius.Compute/containers", gomock.Any(), false).
		DoAndReturn(func(ctx context.Context, resourceType string, resourceID string, force bool) (bool, error) {
			attempted.Add(1)

			return false, errors.New("recipe failed for " + resourceID)
		}).Times(3)

	resources := newTieredResources(
		"Radius.Compute/containers",
		"Radius.Compute/containers",
		"Radius.Compute/containers",
	)
	tiers := GroupResourcesByDeleteTier(resources)

	err := DeleteResourcesInTiers(t.Context(), client, &output.MockOutput{}, tiers, false)
	require.Error(t, err)

	require.Equal(t, int32(3), attempted.Load(), "a failure must not abandon the deletes beside it")
	for i := range resources {
		require.Contains(t, err.Error(), fmt.Sprintf("resource%d", i), "every failure must be reported")
	}
}

// TestDeleteResourcesInTiers_StopsAfterFailedTier asserts that a later tier is skipped once a tier
// has failed. The resources that failed may still depend on it.
func TestDeleteResourcesInTiers_StopsAfterFailedTier(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := clients.NewMockApplicationsManagementClient(ctrl)

	client.EXPECT().
		DeleteResource(gomock.Any(), "Radius.Compute/containers", gomock.Any(), false).
		Return(false, errors.New("recipe failed")).Times(1)

	// No expectation is registered for the environment, so deleting it would fail the test.
	resources := newTieredResources("Radius.Compute/containers", "Radius.Core/environments")
	tiers := GroupResourcesByDeleteTier(resources)

	err := DeleteResourcesInTiers(t.Context(), client, &output.MockOutput{}, tiers, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "recipe failed")
}

// TestDeleteResourcesInTiers_BoundsConcurrency asserts the fan-out cap. Each delete holds a
// long-running operation poller open against the server.
func TestDeleteResourcesInTiers_BoundsConcurrency(t *testing.T) {
	t.Parallel()

	const resourceCount = maxParallelDeletes * 2

	ctrl := gomock.NewController(t)
	client := clients.NewMockApplicationsManagementClient(ctrl)

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	// Released once the cap has been reached, so that the deletes genuinely overlap without
	// depending on timing.
	capReached := make(chan struct{})
	var closeOnce sync.Once

	client.EXPECT().
		DeleteResource(gomock.Any(), gomock.Any(), gomock.Any(), false).
		DoAndReturn(func(ctx context.Context, resourceType string, resourceID string, force bool) (bool, error) {
			current := inFlight.Add(1)
			for {
				observed := maxInFlight.Load()
				if current <= observed || maxInFlight.CompareAndSwap(observed, current) {
					break
				}
			}

			if current >= maxParallelDeletes {
				closeOnce.Do(func() { close(capReached) })
			}

			<-capReached
			inFlight.Add(-1)

			return true, nil
		}).Times(resourceCount)

	resourceTypes := make([]string, resourceCount)
	for i := range resourceTypes {
		resourceTypes[i] = "Radius.Compute/containers"
	}

	tiers := GroupResourcesByDeleteTier(newTieredResources(resourceTypes...))

	err := DeleteResourcesInTiers(t.Context(), client, &output.MockOutput{}, tiers, false)
	require.NoError(t, err)

	require.Equal(t, int32(maxParallelDeletes), maxInFlight.Load(),
		"deletes should saturate the concurrency limit without exceeding it")
}

// TestDeleteResourcesInTiers_FailsOnUnaddressableResources asserts that a resource which cannot be
// addressed fails the delete rather than being skipped.
//
// The caller deletes the resource group once the tiers succeed, and the server does not reject the
// deletion of a non-empty group. Treating an undeletable resource as success would therefore delete
// the group and orphan that resource, with nothing left pointing at it.
func TestDeleteResourcesInTiers_FailsOnUnaddressableResources(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := clients.NewMockApplicationsManagementClient(ctrl)

	// The addressable resource beside it is still deleted: the goal is to report the problem, not
	// to abandon the rest of the tier.
	client.EXPECT().
		DeleteResource(gomock.Any(), "Radius.Compute/containers", gomock.Any(), false).
		Return(true, nil).Times(1)

	resources := append(
		newTieredResources("Radius.Compute/containers"),
		generated.GenericResource{Name: to.Ptr("no-id")},
	)

	out := &output.MockOutput{}
	err := DeleteResourcesInTiers(t.Context(), client, out, GroupResourcesByDeleteTier(resources), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no-id")

	require.Contains(t, out.Writes, output.LogOutput{
		Format: MsgSkippingResource,
		Params: []any{"no-id"},
	})
}

// TestDeleteResourcesInTiers_EmptyTiersSucceed asserts the no-op case, which is what an empty
// resource group produces.
func TestDeleteResourcesInTiers_EmptyTiersSucceed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := clients.NewMockApplicationsManagementClient(ctrl)

	out := &output.MockOutput{}
	err := DeleteResourcesInTiers(t.Context(), client, out, GroupResourcesByDeleteTier(nil), false)
	require.NoError(t, err)
	require.Empty(t, out.Writes)
}
