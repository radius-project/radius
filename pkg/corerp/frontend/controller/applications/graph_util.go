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

package applications

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	"github.com/go-openapi/jsonpointer"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	connectionsPath = "/properties/connections"
	portsPath       = "/properties/container/ports"
)

// listAllResourcesByApplication takes a context, applicationID and clientOptions
// and returns a slice of GenericResources in the application and also an error if one occurs.
func listAllResourcesByApplication(ctx context.Context, applicationID resources.ID, clientOptions *policy.ClientOptions) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range resourceTypesList {
		resourceList, err := listAllResourcesOfTypeInApplication(ctx, applicationID, resourceType, clientOptions)
		if err != nil {
			return nil, err
		}
		results = append(results, resourceList...)
	}
	return results, nil
}

// listAllResourcesOfTypeInApplication takes in a context, an application ID and a
// resource type and returns a slice of GenericResources of that type in the application and an error if one occurs.
func listAllResourcesOfTypeInApplication(ctx context.Context, applicationID resources.ID, resourceType string, clientOptions *policy.ClientOptions) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	resourceList, err := listAllResourcesByType(ctx, applicationID.RootScope(), resourceType, clientOptions)
	if err != nil {
		return nil, err
	}
	applicationName := applicationID.Name()
	for _, resource := range resourceList {
		isResourceWithApplication := isResourceInApplication(ctx, resource, applicationName)
		if isResourceWithApplication {
			results = append(results, resource)
		}
	}
	return results, nil
}

// listAllResourcesByType takes in a context, a root scope and a resource type and returns a slice of GenericResources and an error if one occurs.
func listAllResourcesByType(ctx context.Context, rootScope string, resourceType string, clientOptions *policy.ClientOptions) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}

	client, err := generated.NewGenericResourcesClient(rootScope, resourceType, &aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return []generated.GenericResource{}, err
	}
	pager := client.NewListByRootScopePager(&generated.GenericResourcesClientListByRootScopeOptions{})

	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return []generated.GenericResource{}, err
		}
		resourceList := nextPage.GenericResourcesList.Value
		for _, application := range resourceList {
			results = append(results, *application)
		}
	}
	return results, nil
}

// isResourceInApplication takes in a context, a GenericResource and an application name and returns
// a boolean value indicating whether the resource is in the application or not.
func isResourceInApplication(ctx context.Context, resource generated.GenericResource, applicationName string) bool {
	obj, found := resource.Properties["application"]
	// A resource may not have an application associated with it.
	if !found {
		return false
	}

	associatedAppID, ok := obj.(string)
	if !ok || associatedAppID == "" {
		return false
	}

	idParsed, err := resources.ParseResource(associatedAppID)
	if err != nil {
		return false
	}

	if strings.EqualFold(idParsed.Name(), applicationName) {
		return true
	}

	return false
}

// listAllResourcesByEnvironment takes in a context, an environment ID and a
// resource type and returns a slice of GenericResources and an error if one occurs.
func listAllResourcesByEnvironment(ctx context.Context, environmentID resources.ID, clientOptions *policy.ClientOptions) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range resourceTypesList {
		resourceList, err := listAllResourcesOfTypeInEnvironment(ctx, environmentID, resourceType, clientOptions)
		if err != nil {
			return nil, err
		}
		results = append(results, resourceList...)
	}

	return results, nil
}

// listAllResourcesOfTypeInEnvironment takes in a context, an environment ID and a
// resource type and returns a slice of GenericResources and an error if one occurs.
func listAllResourcesOfTypeInEnvironment(ctx context.Context, environmentID resources.ID, resourceType string, clientOptions *policy.ClientOptions) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	resourceList, err := listAllResourcesByType(ctx, environmentID.RootScope(), resourceType, clientOptions)
	if err != nil {
		return nil, err
	}
	for _, resource := range resourceList {
		isResourceInEnvironment := isResourceInEnvironment(ctx, resource, environmentID.Name())
		if isResourceInEnvironment {
			results = append(results, resource)
		}
	}
	return results, nil
}

// isResourceInEnvironment takes in a context, a GenericResource and an environment name and returns
// a boolean value indicating whether the resource is in the environment or not.
func isResourceInEnvironment(ctx context.Context, resource generated.GenericResource, environmentName string) bool {
	obj, found := resource.Properties["environment"]
	// A resource may not have an environment associated with it.
	if !found {
		return false
	}

	associatedEnvID, ok := obj.(string)
	if !ok || associatedEnvID == "" {
		return false
	}

	idParsed, err := resources.ParseResource(associatedEnvID)
	if err != nil {
		return false
	}

	if strings.EqualFold(idParsed.Name(), environmentName) {
		return true
	}

	return false
}

// computeGraph constructs an application graph from the given application and environment resources.
//
// This function does not return errors and will ignore missing or corrupted data. It is expected that the caller
// will display the results to a human user, so rather than failing to computeGraph the graph, we will return partial
// results. Each ApplicationGraphResource will have a provisioning state that indicates whether the resource
// was successfully processed or not.
func computeGraph(applicationName string, applicationResources []generated.GenericResource, environmentResources []generated.GenericResource) *corerpv20231001preview.ApplicationGraphResponse {
	if applicationResources == nil && environmentResources == nil {
		return &corerpv20231001preview.ApplicationGraphResponse{Resources: []*corerpv20231001preview.ApplicationGraphResource{}}
	}

	// The first step is to figure out what resources are part of the application.
	//
	// Since Radius has both application-scoped and environment-scoped resources we need to merge the two lists
	// and take care to annotate each resource with whether it's part of the application or not. Any application-scoped
	// resource will be part of the environment, so we also filter duplicates.
	//
	// We produce two data-structures:
	// - resources (all "known" resources) using the API wire format
	// - resourcesByIDInApplication (a map of resource ID to whether the resource is part of the application).
	//
	// This allows us to reason about three sets of resources:
	// - Resources that belong to the application: resourcesByIDInApplication[id] == true
	// - Resources that are "known" but don't belong to the application: resourcesByIDInApplication[id] == false
	// - Resources that "external":  ok, _ := resourcesByIDInApplication[id]; !ok (not in the map)
	resources := []generated.GenericResource{}
	resourcesByIDInApplication := map[string]bool{}

	for _, resource := range applicationResources {
		resources = append(resources, resource)

		// Application-scoped resources are by definition "in" the application
		resourcesByIDInApplication[to.String(resource.ID)] = true
	}

	for _, resource := range environmentResources {
		_, found := resourcesByIDInApplication[to.String(resource.ID)]
		if found {
			// Appears in both application and environment lists, avoid duplicates.
			continue
		}

		// This is an environment-scoped resource. We need to process the connections
		// to determine if it's part of the application.
		resources = append(resources, resource)
		resourcesByIDInApplication[to.String(resource.ID)] = false
	}

	// Next we need to process each entry in the resources list and build up the application graph.
	applicationGraphResourcesByID := map[string]corerpv20231001preview.ApplicationGraphResource{}
	for _, resource := range resources {
		applicationGraphResource := applicationGraphResourceFromID(*resource.ID)
		if applicationGraphResource == nil {
			continue // Invalid resource ID, skip
		}

		//update provisioning state
		provisioningState, ok := resource.Properties["provisioningState"]
		if ok {
			// ignore err since in that case we just default to "succeeded"
			state, _ := provisioningState.(string)
			applicationGraphResource.ProvisioningState = &state
		}

		connections := connectionsFromAPIData(resource, resources)          // Outbound connections based on 'connections'
		connections = append(connections, providesFromAPIData(resource)...) // Inbound connections based on 'provides'

		sort.Slice(connections, func(i, j int) bool {
			return to.String(connections[i].ID) < to.String(connections[j].ID)
		})

		applicationGraphResource.Connections = connections
		applicationGraphResource.OutputResources = outputResourcesFromAPIData(resource)

		applicationGraphResourcesByID[*resource.ID] = *applicationGraphResource
	}

	// Now we've massaged the data into a format we like, but we still don't have a comprehensive list
	// of everything in the application. There are two categories we need to address:
	//
	// - Cloud resources that are referenced by an application-scoped resource (recursively).
	// - Environment-scoped resources referenced by an application-scoped resource (recursively).
	//
	// To do this we'll do a breadth first search of the graph starting with the application-scoped resources. We can
	// use `resourcesByIDInApplication` to track the nodes we have already visited and prevent duplicates. It's important
	// to note that a resource never transitions from being "in" the application to being "out", so we only need to visit
	// each node once.
	//
	// Since we're exploring a graph, we're using a "queue" to process resources in a breadth-first manner.
	queue := []string{}

	// While we do this we'll also build up a bi-directional adjency map of connections so we can resolve in-bound
	// connections.
	connectionsBySource := map[string][]corerpv20231001preview.ApplicationGraphConnection{}
	connectionsByDestination := map[string][]corerpv20231001preview.ApplicationGraphConnection{}

	// First process add resources we *know* are in the application to the queue. As we explore the graph we'll
	// visit resources outside the application if necessary.
	for _, entry := range applicationGraphResourcesByID {
		if resourcesByIDInApplication[to.String(entry.ID)] {
			queue = append(queue, to.String(entry.ID))
		}
	}

	for len(queue) > 0 {
		// Pop!
		id := queue[0]
		queue = queue[1:]
		entry := applicationGraphResourcesByID[id]

		for _, connection := range entry.Connections {
			otherID := *connection.ID
			direction := connection.Direction

			// For each connection let's make sure the destination is also part of the application graph. This handles
			// The two cases mentioned above.
			inApplication, found := resourcesByIDInApplication[otherID]
			if !found {
				// Case 1) This is a cloud resource that is referenced by an application-scoped resource.
				//
				// Add it to the queue for processing, and include it in the application.
				//
				// Since this is a cloud resource we need to create a new entry in 'resourceEntriesByID'.
				queue = append(queue, otherID)
				applicationGraphResourcesByID[otherID] = *applicationGraphResourceFromID(otherID)
				resourcesByIDInApplication[otherID] = true
			}
			if !inApplication {
				// Case 2) This is an environment-scoped resource that is referenced by an application-scoped resource.
				//
				// Add it to the queue for processing, and include it in the application.
				//
				// This resource may have connections to other resources, since we are adding it to the queue, that guarantees
				// it will also have its connections processed.
				//
				// Since this is an environment-scoped resource it should already have an entry in 'resourceEntriesByID'.
				queue = append(queue, otherID)
				resourcesByIDInApplication[otherID] = true
			}

			// Note the connection in both directions.
			//id is the source from which the connections in connectionsBySource go out
			if *direction == corerpv20231001preview.DirectionOutbound { // we are dealing with a relation formed by "connection"
				connectionsBySource[id] = append(connectionsBySource[id], *connection)
				dir := corerpv20231001preview.DirectionInbound
				//otherID is the destination to the connections in connectionsByDestination
				connectionInbound := corerpv20231001preview.ApplicationGraphConnection{
					ID:        &id,
					Direction: &dir, //Direction is set with respect to Resource defining this connection
				}
				connectionsByDestination[otherID] = append(connectionsByDestination[otherID], connectionInbound)
			} else {
				// We dont have to note anything in connectionsOutbound because 'provides' allows us to determine just the
				// missing inbound connections to HTTPRoutes. All outbound connections are already captured by 'connections'.
				connectionsBySource[otherID] = append(connectionsBySource[otherID], *connection)
			}
		}
	}

	// Now we know *fully* the set of resources in the application. We can build the final graph.
	graph := corerpv20231001preview.ApplicationGraphResponse{Resources: []*corerpv20231001preview.ApplicationGraphResource{}}

	for id, inApplication := range resourcesByIDInApplication {
		if !inApplication {
			continue // Not in application, skip
		}

		// We have one job left to do, which is to update the inbound connections. The outbound connections
		// were already done.
		entry := applicationGraphResourcesByID[id]
		connectionsIn := connectionsByDestination[id]

		entryConnections := make([]*corerpv20231001preview.ApplicationGraphConnection, len(connectionsIn))
		for i, conn := range connectionsIn {
			entryConnections[i] = &conn
		}
		entry.Connections = append(entry.Connections, entryConnections...)

		// Print connections in stable order.
		sort.Slice(entry.Connections, func(i, j int) bool {
			// Connections are guaranteed to have a name.
			return to.String(entry.Connections[i].ID) < to.String(entry.Connections[j].ID)
		})

		graph.Resources = append(graph.Resources, &entry)
	}
	return &graph
}

// applicationGraphResourceFromID creates a applicationGraphResource from a resource ID.
func applicationGraphResourceFromID(id string) *corerpv20231001preview.ApplicationGraphResource {
	application, err := resources.ParseResource(id)
	if err != nil {
		return nil // Invalid resource ID, skip
	}

	return &corerpv20231001preview.ApplicationGraphResource{
		ID:                to.Ptr(id),
		Name:              to.Ptr(application.Name()),
		Type:              to.Ptr(application.Type()),
		ProvisioningState: to.Ptr(string(v1.ProvisioningStateSucceeded)),
	}
}

// outputResourceEntryFromID creates a outputResourceEntry from a resource ID.
func outputResourceEntryFromID(id resources.ID) corerpv20231001preview.ApplicationGraphOutputResource {
	return corerpv20231001preview.ApplicationGraphOutputResource{
		ID:   to.Ptr(id.String()),
		Name: to.Ptr(id.Name()),
		Type: to.Ptr(id.Type()),
	}
}

// outputResourcesFromAPIData processes the generic resource representation returned by the Radius API
// and produces a list of output resources.
func outputResourcesFromAPIData(resource generated.GenericResource) []*corerpv20231001preview.ApplicationGraphOutputResource {
	// We need to access the output resources in a weakly-typed way since the data type we're
	// working with is a property bag.
	//
	// Any Radius resource type that supports output resources uses the following property path to return them.
	p, err := jsonpointer.New("/properties/status/outputResources")
	if err != nil {
		// This should never fail since we're hard-coding the path.
		panic("parsing JSON pointer should not fail: " + err.Error())
	}

	raw, _, err := p.Get(&resource)
	if err != nil {
		// Not found, this is fine.
		return []*corerpv20231001preview.ApplicationGraphOutputResource{}
	}

	ors, ok := raw.([]any)
	if !ok {
		// Not an array, this is fine.
		return []*corerpv20231001preview.ApplicationGraphOutputResource{}
	}

	// The data is returned as an array of JSON objects. We need to convert each object from a map[string]any
	// to the strongly-typed format we understand.
	//
	// If we encounter an error processing this data, just and an "invalid" output resource entry.
	entries := []*corerpv20231001preview.ApplicationGraphOutputResource{}
	for _, or := range ors {
		// This is the wire format returned by the API for an output resource.
		type outputResourceWireFormat struct {
			ID resources.ID `json:"id"`
		}

		data := outputResourceWireFormat{}
		err = toStronglyTypedData(or, &data)
		if err != nil {
			continue
		}

		// Now build the entry from the API data
		entry := outputResourceEntryFromID(data.ID)

		entries = append(entries, &entry)
	}

	// Produce a stable output
	sort.Slice(entries, func(i, j int) bool {
		if to.String(entries[i].Type) != to.String(entries[j].Type) {
			return to.String(entries[i].Type) < to.String(entries[j].Type)
		}
		if to.String(entries[i].Name) != to.String(entries[j].Name) {
			return to.String(entries[i].Name) < to.String(entries[j].Name)
		}
		return to.String(entries[i].ID) < to.String(entries[j].ID)

	})

	return entries
}

// connectionsFromAPIData resolves the outbound connections for a resource from the generic resource representation.
// For example if the resource is an 'Applications.Core/container' then this function can find it's connections
// to other resources like databases. Conversely if the resource is a database then this function
// will not find any connections (because they are inbound). Inbound connections are processed later.
func connectionsFromAPIData(resource generated.GenericResource, allResources []generated.GenericResource) []*corerpv20231001preview.ApplicationGraphConnection {
	// We need to access the connections in a weakly-typed way since the data type we're
	// working with is a property bag.
	//
	// Any Radius resource type that supports connections uses the following property path to return them.
	p, err := jsonpointer.New(connectionsPath)
	if err != nil {
		// This should never fail since we're hard-coding the path.
		panic("parsing JSON pointer should not fail: " + err.Error())
	}

	raw, _, err := p.Get(&resource)
	if err != nil {
		// Not found, this is fine.
		return []*corerpv20231001preview.ApplicationGraphConnection{}
	}

	connections, ok := raw.(map[string]any)
	if !ok {
		// Not a map of objects, this is fine.
		return []*corerpv20231001preview.ApplicationGraphConnection{}
	}

	// The data is returned as a map of JSON objects. We need to convert each object from a map[string]any
	// to the strongly-typed format we understand.
	//
	// If we encounter an error processing this data, just skip "invalid" connection entry.
	entries := []*corerpv20231001preview.ApplicationGraphConnection{}
	for _, connection := range connections {
		dir := corerpv20231001preview.DirectionInbound
		data := corerpv20231001preview.ConnectionProperties{}
		err := toStronglyTypedData(connection, &data)
		if err == nil {
			sourceID, _ := findSourceResource(to.String(data.Source), allResources)

			entries = append(entries, &corerpv20231001preview.ApplicationGraphConnection{
				ID:        to.Ptr(sourceID),
				Direction: to.Ptr(dir),
			})
		}
	}

	// Produce a stable output
	sort.Slice(entries, func(i, j int) bool {
		return to.String(entries[i].ID) < to.String(entries[j].ID)
	})

	return entries
}

// findSourceResource finds resource id by using source string by the following criteria:
// 1. It immediately returns return the resource ID if the source is a resource ID.
// 2. Parse the hostname and look up the hostname from the resource list if the source is an URL.
// 3. Otherwise, return the original source string with false boolean value.
func findSourceResource(source string, allResources []generated.GenericResource) (string, bool) {
	// 1. Return if the source is a resource ID
	id, err := resources.Parse(source)
	if err == nil && id.IsResource() {
		return id.String(), true
	}

	// 2. Parse hostname from source and look up hostname from resource list.
	orig := source

	// Add "//" to source to enable url.Parse to parse source correctly if the scheme is not given.
	if !strings.Contains(source, "//") {
		source = "//" + source
	}

	sourceURL, err := url.Parse(source)
	if err == nil {
		for _, resource := range allResources {
			if to.String(resource.Name) == sourceURL.Hostname() {
				return to.String(resource.ID), true
			}
		}
	}

	// 3. Return the original source string with false boolean value.
	return orig, false
}

// providesFromAPIData is specifically to support HTTPRoute.
func providesFromAPIData(resource generated.GenericResource) []*corerpv20231001preview.ApplicationGraphConnection {
	// Any Radius resource type that exposes a port uses the following property path to return them.
	// The port may have a 'provides' attribute that specifies a httproute.
	// This route should be parsed to find the connections between containers.
	// For example, if container A provides a route and container B consumes it,
	// then we have port.provides in container A and container.connection in container B.
	// This gives us the connection: container A --> route R --> container B.
	// Without parsing the 'provides' attribute, we would miss the connection between container A and route R.

	p, err := jsonpointer.New(portsPath)
	if err != nil {
		// This should never fail since we're hard-coding the path.
		panic("parsing JSON pointer should not fail: " + err.Error())
	}

	raw, _, err := p.Get(&resource)
	if err != nil {
		// Not found, this is fine.
		return []*corerpv20231001preview.ApplicationGraphConnection{}
	}

	connections, ok := raw.(map[string]any)
	if !ok {
		// Not a map of objects, this is fine.
		return []*corerpv20231001preview.ApplicationGraphConnection{}
	}

	// The data is returned as a map of JSON objects. We need to convert each object from a map[string]any
	// to the strongly-typed format we understand.
	//
	// If we encounter an error processing this data, just skip "invalid" connection entry.
	entries := []*corerpv20231001preview.ApplicationGraphConnection{}
	for _, connection := range connections {
		dir := corerpv20231001preview.DirectionOutbound
		data := corerpv20231001preview.ContainerPortProperties{}
		err := toStronglyTypedData(connection, &data)
		if err == nil {
			if to.String(data.Provides) == "" {
				continue
			}

			entries = append(entries, &corerpv20231001preview.ApplicationGraphConnection{
				ID:        data.Provides,
				Direction: to.Ptr(dir),
			})
		}
	}

	// Produce a stable output
	sort.Slice(entries, func(i, j int) bool {
		return to.String(entries[i].ID) < to.String(entries[j].ID)
	})

	return entries
}

// toStronglyTypedData uses JSON marshalling and unmarshalling to convert a weakly-typed
// representation to a strongly-typed one.
func toStronglyTypedData(data any, result any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}

	err = json.Unmarshal(b, result)
	if err != nil {
		return err
	}

	return nil
}
