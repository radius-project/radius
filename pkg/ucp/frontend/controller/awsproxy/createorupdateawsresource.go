// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	http "net/http"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/wI2L/jsondiff"
	"golang.org/x/exp/slices"
)

type ResourceTypeSchema struct {
	Properties           map[string]interface{} `json:"properties,omitempty"`
	ReadOnlyProperties   []string               `json:"readOnlyProperties,omitempty"`
	CreateOnlyProperties []string               `json:"createOnlyProperties,omitempty"`
}

var _ armrpc_controller.Controller = (*CreateOrUpdateAWSResource)(nil)

// CreateOrUpdateAWSResource is the controller implementation to create/update an AWS resource.
type CreateOrUpdateAWSResource struct {
	ctrl.BaseController
}

// NewCreateOrUpdateAWSResource creates a new CreateOrUpdateAWSResource.
func NewCreateOrUpdateAWSResource(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateAWSResource{ctrl.NewBaseController(opts)}, nil
}

func (p *CreateOrUpdateAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	cloudControlClient, cloudFormationClient, resourceType, id, err := ParseAWSRequest(ctx, p.Options, req)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()

	body := map[string]interface{}{}
	err = decoder.Decode(&body)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: "failed to read request body",
			},
		}

		response := armrpc_rest.NewBadRequestARMResponse(e)
		err = response.Apply(ctx, w, req)
		if err != nil {
			return nil, err
		}
	}

	properties := map[string]interface{}{}
	obj, ok := body["properties"]
	if ok {
		pp, ok := obj.(map[string]interface{})
		if ok {
			properties = pp
		}
	}

	// Create and update work differently for AWS - we need to know if the resource
	// we're working on exists already.

	existing := true
	getResponse, err := cloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awserror.IsAWSResourceNotFound(err) {
		existing = false
	} else if err != nil {
		return awserror.HandleAWSError(err)
	}

	var operation uuid.UUID
	desiredState, err := json.Marshal(properties)
	if err != nil {
		return awserror.HandleAWSError(err)
	}

	// AWS doesn't return the resource state as part of the cloud-control operation. Let's
	// simulate that here.
	responseProperties := map[string]interface{}{}
	if getResponse != nil {
		err = json.Unmarshal([]byte(*getResponse.ResourceDescription.Properties), &responseProperties)
		if err != nil {
			return awserror.HandleAWSError(err)
		}
	}

	// Properties specified by users take precedence
	for k, v := range properties {
		responseProperties[k] = v
	}

	if existing {
		// Get resource type schema
		describeTypeOutput, err := cloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
			Type:     types.RegistryTypeResource,
			TypeName: aws.String(resourceType),
		})
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		var resourceTypeSchema ResourceTypeSchema
		err = json.Unmarshal([]byte(*describeTypeOutput.Schema), &resourceTypeSchema)
		if err != nil {
			return awserror.HandleAWSError(err)
		}
		var readOnlyProperties = mapValues(resourceTypeSchema.ReadOnlyProperties, removePropertyKeywordFromString)
		var createOnlyProperties = mapValues(resourceTypeSchema.CreateOnlyProperties, removePropertyKeywordFromString)

		var currentStateObject map[string]interface{}
		err = json.Unmarshal([]byte(*getResponse.ResourceDescription.Properties), &currentStateObject)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		var flattenedCurrentStateObject = flattenProperties(currentStateObject)

		var desiredStateObject map[string]interface{}
		err = json.Unmarshal(desiredState, &desiredStateObject)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		var flattenedDesiredStateObject = flattenProperties(desiredStateObject)

		var flattenedGoalStateObject = map[string]interface{}{}

		// Add read-only and create-only properties from the current state to the goal state
		for k, v := range flattenedCurrentStateObject {
			if slices.Contains(readOnlyProperties, k) || slices.Contains(createOnlyProperties, k) {
				flattenedGoalStateObject[k] = v
			}
		}

		// Add (or overwrite) properties from desired state to the goal state
		for k, v := range flattenedDesiredStateObject {
			// Don't add create-only properties (for idempotency)
			if !slices.Contains(createOnlyProperties, k) {
				flattenedGoalStateObject[k] = v
			}
		}

		// Convert current and goal states back into unflattened maps
		unflattenedCurrentStateObject := unflattenProperties(flattenedCurrentStateObject)
		unflattenedGoalStateObject := unflattenProperties(flattenedGoalStateObject)

		// Marshal current state
		currentState, err := json.Marshal(unflattenedCurrentStateObject)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// Marshal goal state
		goalState, err := json.Marshal(unflattenedGoalStateObject)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// Calculate the patch based on the current state and the goal state
		patch, err := jsondiff.CompareJSON(currentState, goalState)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// Call update only if the patch is not empty
		if len(patch) > 0 {
			marshaled, err := json.Marshal(&patch)
			if err != nil {
				return awserror.HandleAWSError(err)
			}

			response, err := cloudControlClient.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
				TypeName:      &resourceType,
				Identifier:    aws.String(id.Name()),
				PatchDocument: aws.String(string(marshaled)),
			})
			if err != nil {
				return awserror.HandleAWSError(err)
			}

			operation, err = uuid.Parse(*response.ProgressEvent.RequestToken)
			if err != nil {
				return awserror.HandleAWSError(err)
			}
		} else {
			// mark provisioning state as succeeded here
			// and return 200, telling the deployment engine that the resource has already been created
			responseProperties["provisioningState"] = v1.ProvisioningStateSucceeded
			responseBody := map[string]interface{}{
				"id":         id.String(),
				"name":       id.Name(),
				"type":       id.Type(),
				"properties": responseProperties,
			}

			resp := armrpc_rest.NewOKResponse(responseBody)
			return resp, nil
		}
	} else {
		response, err := cloudControlClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
			TypeName:     &resourceType,
			DesiredState: aws.String(string(desiredState)),
		})
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		operation, err = uuid.Parse(*response.ProgressEvent.RequestToken)
		if err != nil {
			return awserror.HandleAWSError(err)
		}
	}

	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning

	responseBody := map[string]interface{}{
		"id":         id.String(),
		"name":       id.Name(),
		"type":       id.Type(),
		"properties": responseProperties,
	}

	resp := armrpc_rest.NewAsyncOperationResponse(responseBody, "global", 201, id, operation, "", id.RootScope(), p.Options.BasePath)
	return resp, nil
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
					if propertySet, ok := currentState.(map[string]interface{}); ok {
						propertySet[subKey] = v
					}
				} else {
					if _, exists := unflattenedState[subKey]; !exists {
						unflattenedState[subKey] = map[string]interface{}{}
					}

					currentState = unflattenedState[subKey]
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
