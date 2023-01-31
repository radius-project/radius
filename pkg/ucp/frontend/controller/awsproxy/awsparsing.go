// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	awsoperations "github.com/project-radius/radius/pkg/aws/operations"
	"github.com/project-radius/radius/pkg/middleware"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func ParseAWSRequest(ctx context.Context, opts ctrl.Options, r *http.Request) (awsclient.AWSCloudControlClient, awsclient.AWSCloudFormationClient, string, resources.ID, error) {
	if opts.AWSCloudControlClient == nil {
		return nil, nil, "", resources.ID{}, errors.New("AWSCloudControlClient is not set.")
	}
	if opts.AWSCloudFormationClient == nil {
		return nil, nil, "", resources.ID{}, errors.New("AWSCloudFormationClient is not set.")
	}

	path := middleware.GetRelativePath(opts.BasePath, r.URL.Path)
	id, err := resources.ParseByMethod(path, r.Method)
	if err != nil {
		return nil, nil, "", resources.ID{}, err
	}

	resourceType := resources.ToAWSResourceType(id)
	return opts.AWSCloudControlClient, opts.AWSCloudFormationClient, resourceType, id, nil
}

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
