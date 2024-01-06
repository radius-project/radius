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

package bicep

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func TestRunner_extractDestination(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		want    *destination
		wantErr bool
	}{
		{
			name:    "no target",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "no tag",
			target:  "ghcr.io/test-registry/test/repo",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "no repo",
			target:  "ghcr.io/test-registry",
			want:    nil,
			wantErr: true,
		},
		{
			name:   "host docker.io",
			target: "docker.io/repo:tag",
			want: &destination{
				host: "index.docker.io",
				repo: "repo",
				tag:  "tag"},
			wantErr: false,
		},
		{
			name:   "host registry-1.docker.io",
			target: "docker.io/repo:tag",
			want: &destination{
				host: "index.docker.io",
				repo: "repo",
				tag:  "tag"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				Target: tt.target,
			}
			got, err := r.extractDestination()
			if (err != nil) != tt.wantErr {
				t.Errorf("Runner.extractDestination() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Runner.extractDestination() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_pushBlob(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		blob      []byte
		target    oras.Target
		wantDesc  ocispec.Descriptor
		wantErr   bool
	}{
		{
			name:      "push layer blob",
			mediaType: layerMediaType,
			blob:      []byte("layer"),
			target:    memory.New(),
			wantDesc: ocispec.Descriptor{
				MediaType: layerMediaType,
				Digest:    digest.FromBytes([]byte("layer")),
				Size:      int64(len([]byte("layer"))),
			},
			wantErr: false,
		},
		{
			name:      "push config blob",
			mediaType: configMediaType,
			blob:      []byte("config"),
			target:    memory.New(),
			wantDesc: ocispec.Descriptor{
				MediaType: configMediaType,
				Digest:    digest.FromBytes([]byte("config")),
				Size:      int64(len([]byte("config"))),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDesc, err := pushBlob(context.Background(), tt.mediaType, tt.blob, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("pushBlob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotDesc, tt.wantDesc) {
				t.Errorf("pushBlob() = %v, want %v", gotDesc, tt.wantDesc)
			}
		})
	}
}

func Test_generateManifestContent(t *testing.T) {
	tests := []struct {
		name             string
		config           ocispec.Descriptor
		layers           []ocispec.Descriptor
		expectedManifest ocispec.Manifest
		wantErr          bool
	}{
		{
			name: "generate manifest content",
			config: ocispec.Descriptor{
				MediaType: configMediaType,
				Digest:    digest.FromBytes([]byte("config")),
				Size:      int64(len([]byte("config"))),
			},
			layers: []ocispec.Descriptor{
				{
					MediaType: layerMediaType,
					Digest:    digest.FromBytes([]byte("layer")),
					Size:      int64(len([]byte("layer"))),
				},
			},
			expectedManifest: ocispec.Manifest{
				Config: ocispec.Descriptor{
					MediaType: configMediaType,
					Digest:    digest.FromBytes([]byte("config")),
					Size:      int64(len([]byte("config"))),
				},
				Layers: []ocispec.Descriptor{
					{
						MediaType: layerMediaType,
						Digest:    digest.FromBytes([]byte("layer")),
						Size:      int64(len([]byte("layer"))),
					},
				},
				Versioned: specs.Versioned{SchemaVersion: 2},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateManifestContent(tt.config, tt.layers...)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateManifestContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			want, err := json.Marshal(tt.expectedManifest)
			require.NoError(t, err)

			if !reflect.DeepEqual(got, want) {
				t.Errorf("generateManifestContent() = %v, want %v", got, want)
			}
		})
	}
}

func TestRunner_prepareDestination(t *testing.T) {
	tests := []struct {
		name      string
		dest      *destination
		plainHTTP bool
		want      *remote.Repository
		wantErr   bool
	}{
		{
			name: "prepare destination",
			dest: &destination{
				host: "index.docker.io",
				repo: "repo",
				tag:  "tag",
			},
			plainHTTP: false,
			want: &remote.Repository{
				Reference: registry.Reference{
					Registry:   "index.docker.io",
					Repository: "repo",
					Reference:  "tag",
				},
			},
			wantErr: false,
		},
		{
			name: "prepare destination : local registry",
			dest: &destination{
				host: "localhost:8000",
				repo: "repo",
				tag:  "tag",
			},
			plainHTTP: true,
			want: &remote.Repository{
				Reference: registry.Reference{
					Registry:   "localhost:8000",
					Repository: "repo",
					Reference:  "tag",
				},
				PlainHTTP: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				Destination: tt.dest,
				PlainHTTP:   tt.plainHTTP,
			}
			got, err := r.prepareDestination(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Runner.prepareDestination() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			require.Equal(t, tt.want.Reference.Registry, got.Reference.Registry)
			require.Equal(t, tt.want.Reference.Repository, got.Reference.Repository)
			require.Equal(t, tt.want.PlainHTTP, got.PlainHTTP)
		})
	}
}

func TestRunner_Validate(t *testing.T) {
	tests := []radcli.ValidateInput{
		{
			Name:          "No flags",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
			},
		},
		{
			Name: "With file flag but no target flag",
			Input: []string{
				"--file",
				"redis.recipe.bicep",
			},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
			},
		},
		{
			Name: "With file and target flags",
			Input: []string{
				"--file",
				"redis.recipe.bicep",
				"--target",
				"br:ghcr.io/test-registry/test/repo:tag",
			},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
			},
		},
		{
			Name: "With file and target w/o `br` flags",
			Input: []string{
				"--file",
				"redis.recipe.bicep",
				"--target",
				"ghcr.io/test-registry/test/repo:tag",
			},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, tests)
}

type MockErrorResponse struct {
	Method     string
	URL        *url.URL
	StatusCode int
}

// Error implements error.
func (e *MockErrorResponse) Error() string {
	// panic("unimplemented")
	return e.Method
}

func (e *MockErrorResponse) GetMethod() string {
	return e.Method
}

func (e *MockErrorResponse) GetURL() *url.URL {
	return e.URL
}

func (e *MockErrorResponse) GetStatusCode() int {
	return e.StatusCode
}

func TestHandleErrorResponse(t *testing.T) {
	// Test case 1: Unauthorized with ACR info

	httpErr1 := &MockErrorResponse{
		URL:        &url.URL{Host: "myregistry.azurecr.io"},
		StatusCode: http.StatusUnauthorized,
	}

	message1 := "Failure reason A"
	expectedError1 := fmt.Sprintf("%s\nUnauthorized: Please login to %q\nFor more details visit: https://learn.microsoft.com/en-us/azure/container-registry/container-registry-authentication?tabs=azure-cli Cause: \"//myregistry.azurecr.io\": response status code 401: Unauthorized.", message1, httpErr1.URL.Host)

	// Test case 2: Unauthorized without ACR info
	httpErr2 := &MockErrorResponse{
		URL:        &url.URL{Host: "otherregistry.gcr.io"},
		StatusCode: http.StatusUnauthorized,
	}
	message2 := "Failure reason B"
	expectedError2 := fmt.Sprintf("%s\nUnauthorized: Please login to %q Cause: %q: response status code 401: Unauthorized.", message2, httpErr2.URL.Host, "//"+httpErr2.URL.Host)

	// Test case 3: Forbidden
	httpErr3 := &MockErrorResponse{
		URL:        &url.URL{Host: "myregistry.azurecr.io"},
		StatusCode: http.StatusForbidden,
	}
	message3 := "Failure reason C"
	expectedError3 := fmt.Sprintf("%s\nForbidden: You don't have permission to push to %q Cause: %q: response status code 403: Forbidden.", message3, httpErr3.URL.Host, "//"+httpErr3.URL.Host)

	// Test case 4: Not Found
	httpErr4 := &MockErrorResponse{
		URL:        &url.URL{Host: "myregistry.azurecr.io"},
		StatusCode: http.StatusNotFound,
	}
	message4 := "Faure reason D"
	expectedError4 := fmt.Sprintf("%s\nNot Found: Unable to find registry %q Cause: %q: response status code 404: Not Found.", message4, httpErr4.URL.Host, "//"+httpErr4.URL.Host)

	// Test case 5: Other status code
	httpErr5 := &MockErrorResponse{
		URL:        &url.URL{Host: "myregistry.azurecr.io"},
		StatusCode: http.StatusInternalServerError,
	}
	message5 := "Faure reason E"
	expectedError5 := fmt.Sprintf("%s Cause: %q: response status code 500: Internal Server Error.", message5, "//"+httpErr5.URL.Host)

	testCases := []struct {
		httpErr       *MockErrorResponse
		message       string
		expectedError string
	}{
		{httpErr1, message1, expectedError1},
		{httpErr2, message2, expectedError2},
		{httpErr3, message3, expectedError3},
		{httpErr4, message4, expectedError4},
		{httpErr5, message5, expectedError5},
	}

	for _, tc := range testCases {
		t.Run(tc.message, func(t *testing.T) {
			httpErr := &errcode.ErrorResponse{
				Method:     tc.httpErr.GetMethod(),
				URL:        tc.httpErr.GetURL(),
				StatusCode: tc.httpErr.GetStatusCode(),
			}
			result := handleErrorResponse(httpErr, tc.message)
			require.Equal(t, tc.expectedError, result.Error())
		})
	}
}
