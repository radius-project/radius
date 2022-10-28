// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	http "net/http"

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
			return nil, err
		}

		// Generate patch
		currentState := []byte(*getResponse.ResourceDescription.Properties)
		resourceTypeSchema := []byte(*describeTypeOutput.Schema)
		patch, err := generatePatch(currentState, desiredState, resourceTypeSchema)
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

// generatePatch generates a JSON patch based on a given current state, desired state, and resource type schema
func generatePatch(currentState []byte, desiredState []byte, schema []byte) (jsondiff.Patch, error) {
	// See: https://github.com/project-radius/radius/blob/main/docs/adr/ucp/001-aws-resource-updating.md

	// Get the resource type schema - this will tell us the properties of the
	// resource as well as which properties are read-only, create-only, etc.
	var resourceTypeSchema ResourceTypeSchema
	err := json.Unmarshal(schema, &resourceTypeSchema)
	if err != nil {
		return nil, err
	}

	// Get the current state of the resource
	var currentStateObject map[string]interface{}
	err = json.Unmarshal(currentState, &currentStateObject)
	if err != nil {
		return nil, err
	}
	var flattenedCurrentStateObject = flattenProperties(currentStateObject)

	// Get the desired state of the resource
	var desiredStateObject map[string]interface{}
	err = json.Unmarshal(desiredState, &desiredStateObject)
	if err != nil {
		return nil, err
	}
	var flattenedDesiredStateObject = flattenProperties(desiredStateObject)

	// Add read-only and create-only properties from current state to the desired state
	for k, v := range flattenedCurrentStateObject {
		property := fmt.Sprintf("/properties/%s", k)

		// Add the property to the desired state if it is not already set
		if _, exists := flattenedDesiredStateObject[k]; !exists {
			// Only add the property to the desired state if it is read-only or create-only
			isReadOnlyProperty := slices.Contains(resourceTypeSchema.ReadOnlyProperties, property)
			isCreateOnlyProperty := slices.Contains(resourceTypeSchema.CreateOnlyProperties, property)
			if isReadOnlyProperty || isCreateOnlyProperty {
				flattenedDesiredStateObject[k] = v
			}
		}
	}

	// Convert desired patch state back into unflattened object
	unflattenedDesiredStateObject := unflattenProperties(flattenedDesiredStateObject)

	// Marshal desired state into bytes
	updatedDesiredState, err := json.Marshal(unflattenedDesiredStateObject)
	if err != nil {
		return nil, err
	}

	// Calculate the patch based on the current state and the goal state
	return jsondiff.CompareJSON(currentState, updatedDesiredState)
}
