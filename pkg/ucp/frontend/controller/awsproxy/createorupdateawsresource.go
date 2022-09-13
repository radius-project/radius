// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/google/uuid"
	radrprest "github.com/project-radius/radius/pkg/armrpc/rest"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/wI2L/jsondiff"
)

var _ ctrl.Controller = (*CreateOrUpdateAWSResource)(nil)

// CreateOrUpdateAWSResource is the controller implementation to create/update an AWS resource.
type CreateOrUpdateAWSResource struct {
	ctrl.BaseController
}

// NewCreateOrUpdateAWSResource creates a new CreateOrUpdateAWSResource.
func NewCreateOrUpdateAWSResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateAWSResource{ctrl.NewBaseController(opts)}, nil
}

func (p *CreateOrUpdateAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	client, resourceType, id, err := ParseAWSRequest(ctx, p.Options.BasePath, req)

	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()

	body := map[string]interface{}{}
	err = decoder.Decode(&body)
	if err != nil {
		e := rest.ErrorResponse{
			Error: rest.ErrorDetails{
				Code:    rest.Invalid,
				Message: "failed to read request body",
			},
		}

		response := rest.NewBadRequestARMResponse(e)
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
	getResponse, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awserror.IsAWSResourceNotFound(err) {
		existing = false
	} else if err != nil {
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

	responseBody := map[string]interface{}{
		"id":         id.String(),
		"name":       id.Name(),
		"type":       id.Type(),
		"properties": responseProperties,
	}
	var operation uuid.UUID
	if existing {
		desiredState, err := json.Marshal(properties)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// For an existing resource we need to convert the desired state into a JSON-patch document
		patch, err := jsondiff.CompareJSON([]byte(*getResponse.ResourceDescription.Properties), desiredState)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// We need to take out readonly properties. Those are usually not specified by the client, and so
		// our library will generate "remove" operations.
		//
		// Iterate backwards because we're removing items from the array
		for i := len(patch) - 1; i >= 0; i-- {
			if patch[i].Type == "remove" {
				patch = append(patch[:i], patch[i+1:]...)
			}
		}

		marshaled, err := json.Marshal(&patch)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		response, err := client.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
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
		desiredState, err := json.Marshal(properties)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		response, err := client.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
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

	resp := radrprest.NewAsyncOperationResponse(responseBody, "global", 201, id, operation, "", id.RootScope(), p.Options.BasePath)
	return resp, nil
}
