// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func ParseAWSRequest(ctx context.Context, basePath string, r *http.Request) (*cloudcontrol.Client, string, resources.ID, error) {
	// Common parsing in AWS plane requests
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, "", resources.ID{}, err
	}

	cfg.ClientLogMode |= aws.LogRequestWithBody
	cfg.ClientLogMode |= aws.LogResponseWithBody

	client := cloudcontrol.NewFromConfig(cfg)
	path := middleware.GetRelativePath(basePath, r.URL.Path)
	id, err := resources.ParseByMethod(path, r.Method)
	if err != nil {
		return nil, "", resources.ID{}, err
	}

	resourceType := resources.ToARN(id)
	return client, resourceType, id, nil
}
