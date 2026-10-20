/*
Copyright 2026 The Radius Authors.

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

package testutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestGetPodLogsDrainsResponse(t *testing.T) {
	expected := strings.Repeat("a", 1024*1024) + "end-of-logs"
	requests := make(chan *http.Request, 1)
	writeErrors := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests <- request
		_, err := response.Write([]byte(expected))
		writeErrors <- err
	}))
	t.Cleanup(server.Close)

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	actual, err := GetPodLogs(t.Context(), client, "test-namespace", "test-pod", "test-container")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	require.NoError(t, <-writeErrors)

	request := <-requests
	require.Equal(t, "/api/v1/namespaces/test-namespace/pods/test-pod/log", request.URL.Path)
	require.Equal(t, "test-container", request.URL.Query().Get("container"))
}

func TestTestImageReferences(test *testing.T) {
	testCases := []struct {
		name             string
		registry         string
		tag              string
		expectedRegistry string
		expectedTag      string
	}{
		{
			name:             "local default does not follow a release alias",
			expectedRegistry: "ghcr.io/radius-project",
			expectedTag:      "test-local",
		},
		{
			name:             "test run identity is preserved",
			registry:         "localhost:5000",
			tag:              "test-functional-123-2",
			expectedRegistry: "localhost:5000",
			expectedTag:      "test-functional-123-2",
		},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			test.Setenv("DOCKER_REGISTRY", testCase.registry)
			test.Setenv("REL_VERSION", testCase.tag)
			registry, tag := SetDefault()
			require.Equal(test, testCase.expectedRegistry, registry)
			require.Equal(test, testCase.expectedTag, tag)
			require.Equal(test, "magpieimage="+registry+"/magpiego:"+tag, GetMagpieImage())
			require.Equal(test, "magpietag="+tag, GetMagpieTag())
		})
	}
}
