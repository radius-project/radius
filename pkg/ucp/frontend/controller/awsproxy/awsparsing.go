// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	awsoperations "github.com/project-radius/radius/pkg/aws/operations"
	"github.com/project-radius/radius/pkg/middleware"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func ParseAWSRequest(ctx context.Context, opts ctrl.Options, r *http.Request) (awsclient.AWSCloudControlClient, awsclient.AWSCloudFormationClient, string, resources.ID, error) {
	// Common parsing in AWS plane requests
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, nil, "", resources.ID{}, err
	}

	var cloudControlClient awsclient.AWSCloudControlClient
	if opts.AWSCloudControlClient == nil {
		cloudControlClient = cloudcontrol.NewFromConfig(cfg)
	} else {
		cloudControlClient = opts.AWSCloudControlClient
	}

	var cloudFormationClient awsclient.AWSCloudFormationClient
	if opts.AWSCloudControlClient == nil {
		cloudFormationClient = cloudformation.NewFromConfig(cfg)
	} else {
		cloudFormationClient = opts.AWSCloudFormationClient
	}

	path := middleware.GetRelativePath(opts.BasePath, r.URL.Path)
	id, err := resources.ParseByMethod(path, r.Method)
	if err != nil {
		return nil, nil, "", resources.ID{}, err
	}

	resourceType := resources.ToAWSResourceType(id)
	return cloudControlClient, cloudFormationClient, resourceType, id, nil
}

func getPrimaryIdentifiersFromSchema(ctx context.Context, schema string) ([]string, error) {
	schemaObject := map[string]interface{}{}
	err := json.Unmarshal([]byte(schema), &schemaObject)
	if err != nil {
		return nil, err
	}

	primaryIdentifiersObject, exists := schemaObject["primaryIdentifier"]
	if !exists {
		return nil, fmt.Errorf("primaryIdentifier not found in schema")
	}

	primaryIdentifiers, ok := primaryIdentifiersObject.([]interface{})
	if !ok {
		return nil, fmt.Errorf("primaryIdentifier is not an array")
	}

	primaryIdentifiersString := make([]string, len(primaryIdentifiers))
	for i, primaryIdentifier := range primaryIdentifiers {
		primaryIdentifiersString[i] = primaryIdentifier.(string)
	}

	return primaryIdentifiersString, nil
}

func getResourceIDWithMultiIdentifiers(ctx context.Context, properties map[string]interface{}, schema string) (string, error) {
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
			err := fmt.Errorf("mandatory property %s is missing", propertyName)
			return "", err
		}
		resourceID += properties[propertyName].(string) + "|"
	}

	resourceID = strings.TrimSuffix(resourceID, "|")
	return resourceID, nil
}

func readPropertiesFromBody(req *http.Request) (map[string]interface{}, error) {
	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()

	body := map[string]interface{}{}
	err := decoder.Decode(&body)
	if err != nil {
		return nil, err
	}

	properties := map[string]interface{}{}
	obj, ok := body["properties"]
	if ok {
		pp, ok := obj.(map[string]interface{})
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
