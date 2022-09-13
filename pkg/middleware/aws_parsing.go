// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

const (
	AWSClientKey       = "AWSClient"
	AWSResourceTypeKey = "AWSResourceType"
	AWSResourceID      = "AWSResourceID"
)

func AWSParsing(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// Common parsing in AWS plane requests
		ctx := r.Context()
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			handleError(ctx, w, r, err)
			return
		}

		cfg.ClientLogMode |= aws.LogRequestWithBody
		cfg.ClientLogMode |= aws.LogResponseWithBody

		client := cloudcontrol.NewFromConfig(cfg)
		id, err := resources.ParseByMethod(r.URL.Path, r.Method)
		if err != nil {
			handleError(ctx, w, r, err)
			return
		}

		parts := []string{}
		for _, segment := range id.TypeSegments() {
			parts = append(parts, strings.ReplaceAll(strings.ReplaceAll(segment.Type, ".", "::"), "/", "::"))
		}

		resourceType := strings.Join(parts, "::")
		ctx = context.WithValue(ctx, AWSClientKey, client)
		ctx = context.WithValue(ctx, AWSResourceTypeKey, resourceType)
		ctx = context.WithValue(ctx, AWSResourceID, id)
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func handleError(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	errResponse, handleErr := awserror.HandleAWSError(err)
	if handleErr != nil {
		e := rest.ErrorResponse{
			Error: rest.ErrorDetails{
				Code:    "Invalid",
				Message: err.Error(),
			},
		}
		errResponse = rest.NewInternalServerErrorARMResponse(e)
	}
	errResponse.Apply(r.Context(), w, r)
}
