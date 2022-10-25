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
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
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

func lookupAWSResourceSchema(ctx context.Context, cloudFormationClient awsclient.AWSCloudFormationClient, resourceType string) ([]interface{}, error) {
	output, err := cloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(resourceType),
	})
	if err != nil {
		return nil, err
	}

	description := map[string]interface{}{}
	err = json.Unmarshal([]byte(*output.Schema), &description)
	if err != nil {
		return nil, err
	}
	primaryIdentifier := description["primaryIdentifier"].([]interface{})
	return primaryIdentifier, nil
}

func getResourceIDWithMultiIdentifiers(ctx context.Context, cloudFormationClient awsclient.AWSCloudFormationClient, url string, resourceType string, properties map[string]interface{}) (string, error) {
	primaryIdentifiers, err := lookupAWSResourceSchema(ctx, cloudFormationClient, resourceType)
	if err != nil {
		return "", err
	}

	var resourceID string
	for _, pi := range primaryIdentifiers {
		// Primary identifier is of the form /properties/<property-name>
		propertyName := strings.Split(pi.(string), "/")[2]

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

// flattenProperties flattens a state object
func flattenProperties(state map[string]interface{}) map[string]interface{} {
	flattenedState := map[string]interface{}{}

	for k, v := range state {
		// If the value is a map, flatten it
		if reflect.TypeOf(v).Kind() == reflect.Map {
			flattenedSubState := flattenProperties(v.(map[string]interface{}))

			for subK, subV := range flattenedSubState {
				key := k + "/" + subK
				flattenedState[key] = subV
			}
		} else {
			flattenedState[k] = v
		}
	}

	return flattenedState
}

// unflattenProperties unflattens a flattened state object
func unflattenProperties(state map[string]interface{}) map[string]interface{} {
	unflattenedState := map[string]interface{}{}

	for k, v := range state {
		splitPath := strings.Split(k, "/")
		rootKey := splitPath[0]

		if len(splitPath) == 1 {
			unflattenedState[rootKey] = v
		} else {
			var currentState interface{} = unflattenedState
			for i := 0; i < len(splitPath); i++ {
				subKey := splitPath[i]
				if i == len(splitPath)-1 {
					if currentStateMap, ok := currentState.(map[string]interface{}); ok {
						currentStateMap[subKey] = v
					}
				} else {
					if currentStateMap, ok := currentState.(map[string]interface{}); ok {
						if _, exists := currentStateMap[subKey]; !exists {
							currentStateMap[subKey] = map[string]interface{}{}
						}

						currentState = currentStateMap[subKey]
					}
				}
			}
		}
	}

	return unflattenedState
}

// removePropertyKeywordFromString removes "/properties/" from the given string
func removePropertyKeywordFromString(s string) string {
	return strings.Replace(s, "/properties/", "", 1)
}

// mapValues implements the map function on an []string
func mapValues(vs []string, f func(string) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}
