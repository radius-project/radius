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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*ComputeResourceID)(nil)

// CreateOrUpdatePlane is the controller implementation to create/update a UCP plane.
type ComputeResourceID struct {
	ctrl.BaseController
}

// NewCreateOrUpdatePlane creates a new CreateOrUpdatePlane.
func NewComputeResourceID(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ComputeResourceID{ctrl.NewBaseController(opts)}, nil
}

func (p *ComputeResourceID) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	body, err := controller.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}

	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)

	// Lookup resource type schema
	client, _, id, err := ParseAWSRequest(ctx, p.Options, req)
	if err != nil {
		return nil, err
	}

	response, err := client.GetResourceType(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(id.Name()),
	})
	if awscli

	return restResp, nil
}


func getPrimaryIdentifiers(typeName string) []interface{} {
	sess, err := session.NewSession()
	if err != nil {
		fmt.Println("Error creating session ", err)
		return nil
	}
	svc := cloudformation.New(sess, aws.NewConfig().WithRegion("us-west-2"))
	output, err := svc.DescribeType(&cloudformation.DescribeTypeInput{
		TypeName: aws.String(typeName),
		Type:     aws.String("RESOURCE"),
	})
	if err != nil {
		fmt.Printf("Error describing type: %s", err.Error())
		return nil
	}

	description := map[string]interface{}{}
	err = json.Unmarshal([]byte(*output.Schema), &description)
	if err != nil {
		fmt.Printf("Error unmarshalling schema: %s", err.Error())
	}
	primaryIdentifier := description["primaryIdentifier"].([]interface{})
	fmt.Printf("Primary identifier: %v", primaryIdentifier)
	return primaryIdentifier
}