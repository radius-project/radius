/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package servicecontext

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
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

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, strings.ToLower(parsed["Referer"]), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range parsed {
		req.Header.Add(k, v)
	}
	return req, nil
}
