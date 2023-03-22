// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

func TestFromContext(t *testing.T) {
	req, err := getTestHTTPRequest("./testdata/armrpcheaders.json")
	require.NoError(t, err)
	serviceCtx, err := v1.FromARMRequest(req, "", v1.LocationGlobal)
	require.NoError(t, err)
	ctx := context.Background()
	newCtx := v1.WithARMRequestContext(ctx, serviceCtx)

	sCtx := AWSRequestContextFromContext(newCtx)
	require.NotNil(t, sCtx)
	require.Equal(t, "2022-09-01-privatepreview", sCtx.APIVersion)
	require.Equal(t, "AWS::Kinesis::Stream", sCtx.ResourceTypeInAWSFormat())
}

func getTestHTTPRequest(headerFile string) (*http.Request, error) {
	jsonData, err := os.ReadFile(headerFile)
	if err != nil {
		return nil, err
	}

	parsed := map[string]string{}
	if err = json.Unmarshal(jsonData, &parsed); err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, strings.ToLower(parsed["Referer"]), nil)
	for k, v := range parsed {
		req.Header.Add(k, v)
	}
	return req, nil
}
