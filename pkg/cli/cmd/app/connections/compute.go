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

package connections

import (
	"encoding/json"
	"sort"

	"github.com/go-openapi/jsonpointer"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/resourcemodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// compute constructs an application graph from the given application and environment resources.
//
// This function does not return errors and will ignore missing or corrupted data. It is expected that the caller
// will display the results to a human user, so rather than failing to compute the graph, we will return partial
// results with annotations for errors.
//
// NOTE: this code DOES NOT yet process connections involving HTTP routes.
func compute(applicationName string, applicationResources []generated.GenericResource, environmentResources []generated.GenericResource) *applicationGraph {
	// The first step is to figure out what resources are part of the application.
	//
	// Since Radius has both application-scoped and environment-scoped resources we need to merge the two lists
	// and take care to annotate each resource with whether it's part of the application or not. Any application-scoped
	// resource will be part of the application, so we also filter duplicates.
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
		resourcesByIDInApplication[*resource.ID] = true
	}

	for _, resource := range environmentResources {
		_, found := resourcesByIDInApplication[*resource.ID]
		if found {
			// Appears in both application and environment lists, avoid duplicates.
			continue
		}

		// This is an environment-scoped resource. We need to process the connections
		// to determine if it's part of the application.
		resources = append(resources, resource)
		resourcesByIDInApplication[*resource.ID] = false
	}

	// Next we need to process each entry in the resources list and build up the application graph.
	resourceEntriesByID := map[string]resourceEntry{}
	for _, resource := range resources {
		entry := resourceEntryFromID(*resource.ID)
		entry.Connections = connectionsFromAPIData(resource)
		entry.Resources = outputResourcesFromAPIData(resource)

		resourceEntriesByID[*resource.ID] = entry
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
	connectionsBySource := map[string][]connectionEntry{}
	connectionsByDestination := map[string][]connectionEntry{}

	// First process add resources we *know* are in the application to the queue. As we explore the graph we'll
	// visit resources outside the application if necessary.
	for _, entry := range resourceEntriesByID {
		if resourcesByIDInApplication[entry.ID] {
			queue = append(queue, entry.ID)
		}
	}

	for len(queue) > 0 {
		// Pop!
		id := queue[0]
		queue = queue[1:]
		entry := resourceEntriesByID[id]

		for _, connection := range entry.Connections {
			destination := connection.To

			// If the connection has an error, we don't need to process it further.
			if destination.Error != "" {
				continue
			}

			// For each connection let's make sure the destination is also part of the application graph. This handles
			// The two cases mentioned above.
			inApplication, found := resourcesByIDInApplication[destination.ID]
			if !found {
				// Case 1) This is a cloud resource that is referenced by an application-scoped resource.
				//
				// Add it to the queue for processing, and include it in the application.
				//
				// Since this is a cloud resource we need to create a new entry in 'resourceEntriesByID'.
				queue = append(queue, destination.ID)
				resourceEntriesByID[destination.ID] = resourceEntryFromID(destination.ID)
				resourcesByIDInApplication[destination.ID] = true

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
				queue = append(queue, destination.ID)
				resourcesByIDInApplication[destination.ID] = true
			}

			// Note the connection in both directions.
			connectionsBySource[connection.From.ID] = append(connectionsBySource[connection.From.ID], connection)
			connectionsByDestination[connection.To.ID] = append(connectionsByDestination[connection.To.ID], connection)
		}
	}

	// Now we know *fully* the set of resources in the application. We can build the final graph.
	graph := applicationGraph{ApplicationName: applicationName, Resources: map[string]resourceEntry{}}
	for id, inApplication := range resourcesByIDInApplication {
		if !inApplication {
			continue // Not in application, skip
		}

		// We have one job left to do, which is to update the inbound connections. The outbound connections
		// were already done.
		entry := resourceEntriesByID[id]
		connectionsIn := connectionsByDestination[id]
		entry.Connections = append(entry.Connections, connectionsIn...)

		// Print connections in stable order.
		sort.Slice(entry.Connections, func(i, j int) bool {
			// Connections are guaranteed to have a name.
			return entry.Connections[i].Name < entry.Connections[j].Name
		})

		graph.Resources[id] = entry
	}

	return &graph
}

// nodeFromID creates a node from a resource ID.
func nodeFromID(id string) node {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return node{Error: err.Error()}
	}

	return node{
		ID:   id,
		Name: parsed.Name(),
		Type: parsed.Type(),
	}
}

// nodeFromParsedID creates a node from a resource ID.
func nodeFromParsedID(id resources.ID) node {
	return node{
		ID:   id.String(),
		Name: id.Name(),
		Type: id.Type(),
	}
}

// resourceEntryFromID creates a resourceEntry from a resource ID.
func resourceEntryFromID(id string) resourceEntry {
	return resourceEntry{node: nodeFromID(id)}
}

// outputResourceEntryFromID creates a outputResourceEntry from a resource ID.
func outputResourceEntryFromID(id resources.ID) outputResourceEntry {
	entry := outputResourceEntry{node: nodeFromParsedID(id)}
	if len(id.ScopeSegments()) > 0 && id.IsUCPQualfied() {
		entry.Provider = id.ScopeSegments()[0].Type
	} else if len(id.ScopeSegments()) > 0 {
		// Relative Resource ID (ARM)
		entry.Provider = resourcemodel.ProviderAzure
	}

	return entry
}

// outputResourcesFromAPIData processes the generic resource representation returned by the Radius API
// and produces a list of output resources.
func outputResourcesFromAPIData(resource generated.GenericResource) []outputResourceEntry {
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
		return []outputResourceEntry{}
	}

	ors, ok := raw.([]any)
	if !ok {
		// Not an array, this is fine.
		return []outputResourceEntry{}
	}

	// The data is returned as an array of JSON objects. We need to convert each object from a map[string]any
	// to the strongly-typed format we understand.
	//
	// If we enounter an error processing this data, just and an "invalid" output resource entry.
	entries := []outputResourceEntry{}
	for _, or := range ors {
		// This is the wire format returned by the API for an output resource.
		type outputResourceWireFormat struct {
			ID resources.ID `json:"id"`
		}

		data := outputResourceWireFormat{}
		err = toStronglyTypedData(or, &data)
		if err != nil {
			entries = append(entries, outputResourceEntry{node: node{Error: err.Error()}})
			continue
		}

		// Now build the entry from the API data
		entry := outputResourceEntryFromID(data.ID)

		entries = append(entries, entry)
	}

	// Produce a stable output
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Provider != entries[j].Provider {
			return entries[i].Provider < entries[j].Provider
		}
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		if entries[i].ID != entries[j].ID {
			return entries[i].ID < entries[j].ID
		}

		return entries[i].Error < entries[j].Error
	})

	return entries
}

// connectionsFromAPIData resolves the outbound connections for a resource from the generic resource representation.
// For example if the resource is an 'Applications.Core/container' then this function can find it's connections
// to other resources like databases. Conversely if the resource is a database then this function
// will not find any connections (because they are inbound). Inbound connections are processed later.
func connectionsFromAPIData(resource generated.GenericResource) []connectionEntry {
	// We need to access the connections in a weakly-typed way since the data type we're
	// working with is a property bag.
	//
	// Any Radius resource type that supports connections uses the following property path to return them.
	p, err := jsonpointer.New("/properties/connections")
	if err != nil {
		// This should never fail since we're hard-coding the path.
		panic("parsing JSON pointer should not fail: " + err.Error())
	}

	raw, _, err := p.Get(&resource)
	if err != nil {
		// Not found, this is fine.
		return []connectionEntry{}
	}

	connections, ok := raw.(map[string]any)
	if !ok {
		// Not a map of objects, this is fine.
		return []connectionEntry{}
	}

	// The data is returned as a map of JSON objects. We need to convert each object from a map[string]any
	// to the strongly-typed format we understand.
	//
	// If we encounter an error processing this data, just and an "invalid" connection entry.
	entries := []connectionEntry{}
	for name, connection := range connections {

		data := v20220315privatepreview.ConnectionProperties{}
		err := toStronglyTypedData(connection, &data)
		if err != nil {
			entries = append(entries, connectionEntry{
				Name: name,
				From: nodeFromID(*resource.ID),
				To:   node{Error: err.Error()},
			})
			continue
		}

		entries = append(entries, connectionEntry{
			Name: name,
			From: nodeFromID(*resource.ID),
			To:   nodeFromID(*data.Source),
		})
	}

	// Produce a stable output
	sort.Slice(entries, func(i, j int) bool {
		// Connections are guaranteed to have a name.
		return entries[i].Name < entries[j].Name
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
