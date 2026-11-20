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
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	msgSharedOutputResources      = "WARNING: Deleting %s also deletes output resources that other Radius resources use:"
	msgSharedOutputResource       = "  - %s, also used by %s"
	msgSharedOutputResourcesError = "WARNING: Could not check whether other Radius resources use the output resources of %s: %v"
)

// SharedOutputResources lists the output resources of a resource being deleted that other Radius
// resources also have.
type SharedOutputResources struct {
	// ResourceID is the ID of the resource being deleted.
	ResourceID string

	// OutputResources are the shared output resources, in the order the resource lists them.
	OutputResources []SharedOutputResource
}

// SharedOutputResource is one output resource that other Radius resources also have.
type SharedOutputResource struct {
	// Name is the provider resource ID of the output resource, or its output resource ID when it has none.
	Name string

	// UsedBy are the IDs of the other Radius resources that have the output resource.
	UsedBy []string
}

type outputResource struct {
	id                 string
	providerResourceID string
	radiusManaged      bool
}

// FindSharedOutputResources returns, for each resource in deleting, the Radius-managed output
// resources that a resource in others also has. Resources that appear in deleting are ignored in
// others, so resources deleted together do not warn about each other. Output resources that Radius
// does not manage are skipped on the deleting side, because deleting the resource leaves them in place.
//
// Two output resources match when both have a provider resource ID and the IDs are equal. When
// either has none, they match when their output resource IDs are equal.
func FindSharedOutputResources(deleting []generated.GenericResource, others []generated.GenericResource) []SharedOutputResources {
	deletingIDs := map[string]bool{}
	for _, resource := range deleting {
		if resource.ID != nil {
			deletingIDs[strings.ToLower(*resource.ID)] = true
		}
	}

	type otherResource struct {
		id              string
		outputResources []outputResource
	}

	candidates := []otherResource{}
	for _, resource := range others {
		if resource.ID == nil || deletingIDs[strings.ToLower(*resource.ID)] {
			continue
		}

		outputResources := outputResourcesOf(resource)
		if len(outputResources) > 0 {
			candidates = append(candidates, otherResource{id: *resource.ID, outputResources: outputResources})
		}
	}

	results := []SharedOutputResources{}
	for _, resource := range deleting {
		if resource.ID == nil {
			continue
		}

		shared := SharedOutputResources{ResourceID: *resource.ID}
		for _, deleted := range outputResourcesOf(resource) {
			if !deleted.radiusManaged {
				continue
			}

			usedBy := []string{}
			for _, candidate := range candidates {
				if slices.ContainsFunc(candidate.outputResources, deleted.matches) {
					usedBy = append(usedBy, candidate.id)
				}
			}

			if len(usedBy) == 0 {
				continue
			}

			// The same provider resource can appear under several output resource IDs, and each ID
			// can match different owners, so owners are merged into one row per name.
			name := deleted.name()
			index := slices.IndexFunc(shared.OutputResources, func(s SharedOutputResource) bool { return s.Name == name })
			if index < 0 {
				shared.OutputResources = append(shared.OutputResources, SharedOutputResource{Name: name, UsedBy: usedBy})
				continue
			}

			for _, id := range usedBy {
				if !slices.Contains(shared.OutputResources[index].UsedBy, id) {
					shared.OutputResources[index].UsedBy = append(shared.OutputResources[index].UsedBy, id)
				}
			}
		}

		if len(shared.OutputResources) > 0 {
			results = append(results, shared)
		}
	}

	return results
}

// WarnSharedOutputResources logs a warning for each resource in deleting whose Radius-managed output
// resources another Radius resource in the plane also has. The warning is advisory: when the check
// fails, it logs that and returns, so the caller can go on with the delete.
//
// The check lists every resource type in the plane, so it is skipped when no resource in deleting
// has a Radius-managed output resource.
func WarnSharedOutputResources(ctx context.Context, client clients.ApplicationsManagementClient, out output.Interface, subject string, deleting []generated.GenericResource) {
	if !slices.ContainsFunc(deleting, hasRadiusManagedOutputResource) {
		return
	}

	others, err := client.ListResourcesInPlane(ctx)
	if err != nil {
		out.LogInfo(msgSharedOutputResourcesError, subject, err)
		return
	}

	for _, shared := range FindSharedOutputResources(deleting, others) {
		out.LogInfo(msgSharedOutputResources, displayResourceName(shared.ResourceID))
		for _, outputResource := range shared.OutputResources {
			out.LogInfo(msgSharedOutputResource, outputResource.Name, strings.Join(outputResource.UsedBy, ", "))
		}
	}
}

func hasRadiusManagedOutputResource(resource generated.GenericResource) bool {
	return slices.ContainsFunc(outputResourcesOf(resource), func(o outputResource) bool { return o.radiusManaged })
}

// displayResourceName returns "<type>/<name>" for a resource ID, or the ID itself when it cannot be parsed.
func displayResourceName(id string) string {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return id
	}

	return parsed.Type() + "/" + parsed.Name()
}

// outputResourcesOf reads status.outputResources from a resource's properties. Entries without an
// ID or a provider resource ID are skipped.
func outputResourcesOf(resource generated.GenericResource) []outputResource {
	status, ok := resource.Properties["status"].(map[string]any)
	if !ok {
		return nil
	}

	entries, ok := status["outputResources"].([]any)
	if !ok {
		return nil
	}

	results := []outputResource{}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		id, _ := entry["id"].(string)
		providerResourceID, _ := entry["providerResourceId"].(string)
		radiusManaged, _ := entry["radiusManaged"].(bool)
		if id == "" && providerResourceID == "" {
			continue
		}

		results = append(results, outputResource{id: id, providerResourceID: providerResourceID, radiusManaged: radiusManaged})
	}

	return results
}

func (o outputResource) name() string {
	if o.providerResourceID != "" {
		return o.providerResourceID
	}

	return o.id
}

// matches reports whether two output resources refer to the same provider resource. Provider
// resource IDs are compared exactly because some providers, such as AWS, treat them as case
// sensitive. Output resource IDs are UCP IDs, which are case insensitive.
func (o outputResource) matches(other outputResource) bool {
	if o.providerResourceID != "" && other.providerResourceID != "" {
		return o.providerResourceID == other.providerResourceID
	}

	return o.id != "" && strings.EqualFold(o.id, other.id)
}
