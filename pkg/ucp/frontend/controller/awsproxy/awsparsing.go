/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package awsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	awsoperations "github.com/project-radius/radius/pkg/aws/operations"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// getPrimaryIdentifiersFromSchema returns the primaryIdentifier field from the
// provided AWS CloudFormation type schema
func getPrimaryIdentifiersFromSchema(ctx context.Context, schema string) ([]string, error) {
	schemaObject := map[string]any{}
	err := json.Unmarshal([]byte(schema), &schemaObject)
	if err != nil {
		return nil, err
	}

	primaryIdentifiersObject, exists := schemaObject["primaryIdentifier"]
	if !exists {
		return nil, fmt.Errorf("primaryIdentifier not found in schema")
	}

	primaryIdentifiers, ok := primaryIdentifiersObject.([]any)
	if !ok {
		return nil, fmt.Errorf("primaryIdentifier is not an array")
	}

	var primaryIdentifiersString []string
	for _, primaryIdentifier := range primaryIdentifiers {
		primaryIdentifiersString = append(primaryIdentifiersString, primaryIdentifier.(string))
	}

	return primaryIdentifiersString, nil
}

// getPrimaryIdentifierFromMultiIdentifiers returns the primary identifier for the resource
// when provided desired primary identifier values and the resource type schema
func getPrimaryIdentifierFromMultiIdentifiers(ctx context.Context, properties map[string]any, schema string) (string, error) {
	primaryIdentifiers, err := getPrimaryIdentifiersFromSchema(ctx, schema)
	if err != nil {
		return "", err
	}

	var resourceID string
	for _, primaryIdentifier := range primaryIdentifiers {
		// Primary identifier is of the form /properties/<property-name>
		propertyName, err := awsoperations.ParsePropertyName(primaryIdentifier)
		if err != nil {
			return "", err
		}

		if _, ok := properties[propertyName]; !ok {
			// Mandatory property is missing
			err := &awsclient.AWSMissingPropertyError{
				PropertyName: propertyName,
			}
			return "", err
		}
		resourceID += properties[propertyName].(string) + "|"
	}

	resourceID = strings.TrimSuffix(resourceID, "|")
	return resourceID, nil
}

func readPropertiesFromBody(req *http.Request) (map[string]any, error) {
	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()

	body := map[string]any{}
	err := decoder.Decode(&body)
	if err != nil {
		return nil, err
	}

	properties := map[string]any{}
	obj, ok := body["properties"]
	if ok {
		pp, ok := obj.(map[string]any)
		if ok {
			properties = pp
		}
	}
	return properties, nil
}

func computeResourceID(id resources.ID, resourceID string) string {
	computedID := strings.Split(id.String(), "/:")[0] + resources.SegmentSeparator + resourceID
	return computedID
}
