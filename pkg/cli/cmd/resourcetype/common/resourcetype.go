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

package common

import (
	"context"
	"slices"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// ResourceType is used by the CLI for display of resource types.
type ResourceType struct {
	// Name is the fully-qualified name of the resource type.
	Name string
	// Description of the resource type.
	Description string
	// ResourceProviderNamespace is the namespace of the resource provider.
	ResourceProviderNamespace string
	// APIVersions is the list of API versions supported by the resource type.
	APIVersions map[string]*APIVersionProperties
}

// APIVersionProperties is used to store the schema of the resource type for the api version.
type APIVersionProperties struct {
	// Schema is the schema of the resource type.
	Schema map[string]any
}

// ResourceTypeListOutputFormat is used to format the output of the resource type list and create commands.
type ResourceTypeListOutputFormat struct {
	ResourceType
	// APIVersionList is the list of API versions supported by the resource type.
	APIVersionList []string
}

// ResourceTypesForProvider returns a list of resource types for a given provider.
func ResourceTypesForProvider(provider *v20231001preview.ResourceProviderSummary) []ResourceType {
	resourceTypes := []ResourceType{}
	for name, resourceType := range provider.ResourceTypes {
		rt := ResourceType{
			Name:                      *provider.Name + "/" + name,
			ResourceProviderNamespace: *provider.Name,
		}

		if resourceType.Description != nil {
			rt.Description = *resourceType.Description
		}

		rt.APIVersions = make(map[string]*APIVersionProperties)
		for apiVersion, properties := range resourceType.APIVersions {
			rt.APIVersions[apiVersion] = &APIVersionProperties{
				Schema: properties.Schema,
			}
		}

		resourceTypes = append(resourceTypes, rt)
	}
	return resourceTypes
}

// GetResourceTypeTableFormat returns the fields to output from a resource type object.
func GetResourceTypeTableFormat() output.FormatterOptions {
	formatterOptions := GetResourceTypeShowTableFormat()
	formatterOptions.Columns = append(formatterOptions.Columns, output.Column{
		Heading:  "APIVERSION",
		JSONPath: "{ .APIVersionList }",
	})

	return formatterOptions
}

// GetResourceTypeShowTableFormat returns the fields to output from a resource type object for show command.
func GetResourceTypeShowTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "TYPE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "NAMESPACE",
				JSONPath: "{ .ResourceProviderNamespace }",
			},
		},
	}
}

// GetResourceTypeShowSchemaTableFormat returns the fields to output from a resource type schema object for show command.
func GetResourceTypeShowSchemaTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "NAME",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .Type }",
			},
			{
				Heading:  "REQUIRED",
				JSONPath: "{ .IsRequired }",
			},
			{
				Heading:  "READ-ONLY",
				JSONPath: "{ .IsReadOnly }",
			},
			{
				Heading:  "DESCRIPTION",
				JSONPath: "{ .Description }",
			},
		},
	}
}

// GetResourceTypeDetails fetches the details of a resource type from the resource provider.
func GetResourceTypeDetails(ctx context.Context, resourceProviderName string, resourceTypeName string, client clients.ApplicationsManagementClient) (ResourceType, error) {
	resourceProvider, err := client.GetResourceProviderSummary(ctx, "local", resourceProviderName)
	if clients.Is404Error(err) {
		return ResourceType{}, clierrors.Message("The resource provider %q was not found or has been deleted.", resourceProviderName)
	} else if err != nil {
		return ResourceType{}, err
	}

	resourceTypes := ResourceTypesForProvider(&resourceProvider)
	idx := slices.IndexFunc(resourceTypes, func(rt ResourceType) bool {
		return rt.Name == resourceProviderName+"/"+resourceTypeName
	})

	if idx < 0 {
		return ResourceType{}, clierrors.Message("Resource type %q not found in resource provider %q.", resourceTypeName, *resourceProvider.Name)
	}

	return resourceTypes[idx], nil
}
