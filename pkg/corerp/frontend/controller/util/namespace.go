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

package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const (
	// legacyEnvironmentNamespaceField is where Applications.Core/environments stores its namespace.
	legacyEnvironmentNamespaceField = "properties.compute.kubernetes.namespace"

	// environmentNamespaceField is where Radius.Core/environments stores its namespace.
	environmentNamespaceField = "properties.providers.kubernetes.namespace"
)

// FindEnvironmentNamespaceConflict reports whether any other environment in the plane already
// claims the given Kubernetes namespace, returning the conflicting environment's resource ID or
// an empty string when the namespace is free.
//
// The search is plane-wide (recursive across every resource group) and covers both
// Applications.Core/environments and Radius.Core/environments, because a namespace is a
// cluster-scoped resource: two environments that share one would deploy into the same place
// regardless of which resource group or API version declared them. Scoping the search to a single
// resource group, as this check historically did, let callers bypass the constraint just by
// creating a second resource group.
//
// excludeID is the resource ID of the environment being created or updated. It is skipped so that
// re-applying an unchanged environment does not conflict with its own stored record.
//
// This check is best-effort with respect to concurrency: it reads the current state and the caller
// then writes, so two simultaneous creates naming the same namespace can both pass. The database
// enforces uniqueness on resource ID only and cannot express a cross-resource invariant, so
// closing that window would require a separate namespace-claim resource. The window is small (the
// environment PUT is synchronous) and the failure mode is the pre-existing one: a duplicate slips
// through and is caught the next time either environment is written.
func FindEnvironmentNamespaceConflict(ctx context.Context, databaseClient database.Client, planeScope string, namespace string, excludeID string) (string, error) {
	// An environment without a namespace claims nothing. Querying for the empty string would also
	// be meaningless, since MatchesFilters treats a missing field as a non-match.
	if namespace == "" {
		return "", nil
	}

	legacyResult, err := findResourcesRecursive(ctx, planeScope, datamodel.EnvironmentResourceType, legacyEnvironmentNamespaceField, namespace, databaseClient)
	if err != nil {
		return "", err
	}

	for _, item := range legacyResult.Items {
		env := &datamodel.Environment{}
		if err := item.As(env); err != nil {
			return "", err
		}

		if strings.EqualFold(env.ID, excludeID) {
			continue
		}

		// ACI environments run on Azure Container Instances and never occupy a Kubernetes
		// namespace, so they do not conflict.
		if env.Properties.Compute.Kind == rpv1.ACIComputeKind {
			continue
		}

		return env.ID, nil
	}

	result, err := findResourcesRecursive(ctx, planeScope, datamodel.EnvironmentResourceType_v20250801preview, environmentNamespaceField, namespace, databaseClient)
	if err != nil {
		return "", err
	}

	for _, item := range result.Items {
		env := &datamodel.Environment_v20250801preview{}
		if err := item.As(env); err != nil {
			return "", err
		}

		if strings.EqualFold(env.ID, excludeID) {
			continue
		}

		return env.ID, nil
	}

	return "", nil
}

// NamespaceConflictMessage builds the error message returned when a Kubernetes namespace is
// already claimed by another environment. The conflicting environment's ID is included so an
// operator can find and resolve the collision.
func NamespaceConflictMessage(namespace string, conflictingEnvironmentID string) string {
	return fmt.Sprintf("The Kubernetes namespace specified (%s) is already used by another Radius Environment (%s). Specify a unique Kubernetes namespace.", namespace, conflictingEnvironmentID)
}

// NamespaceImmutableMessage builds the error message returned when a request tries to change or
// clear the Kubernetes namespace of an environment that already has one. requestedNamespace is
// empty when the request drops the Kubernetes provider altogether.
func NamespaceImmutableMessage(currentNamespace string, requestedNamespace string) string {
	if requestedNamespace == "" {
		return fmt.Sprintf("The Kubernetes namespace of an existing environment cannot be removed (current namespace: %s). Resources already deployed into that namespace would be orphaned. Create a new environment instead.", currentNamespace)
	}

	return fmt.Sprintf("The Kubernetes namespace of an existing environment cannot be changed (current namespace: %s, requested: %s). Resources already deployed into %s would be orphaned. Create a new environment instead.", currentNamespace, requestedNamespace, currentNamespace)
}
