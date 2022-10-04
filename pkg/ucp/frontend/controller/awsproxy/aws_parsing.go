// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/project-radius/radius/pkg/middleware"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func ParseAWSRequest(ctx context.Context, opts ctrl.Options, r *http.Request) (awsclient.AWSClient, string, resources.ID, error) {
	// Common parsing in AWS plane requests
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, "", resources.ID{}, err
	}

	var client awsclient.AWSClient
	if opts.AWSClient == nil {
		client = cloudcontrol.NewFromConfig(cfg)
	} else {
		client = opts.AWSClient
	}
	path := middleware.GetRelativePath(opts.BasePath, r.URL.Path)
	id, err := resources.ParseByMethod(path, r.Method)
	if err != nil {
		return nil, "", resources.ID{}, err
	}

	resourceType := resources.ToAWSResourceType(id)
	return client, resourceType, id, nil
}
