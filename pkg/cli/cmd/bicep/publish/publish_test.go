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

package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
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

func TestRunner_extractDestination_EnhancedErrors(t *testing.T) {
	tests := []struct {
		name                string
		target              string
		wantErr             bool
		expectedErrContains []string
	}{
		{
			name:    "uppercase in repository name",
			target:  "localhost:5000/myregistry/Data/mySqlDatabases/kubernetes/kubernetesmysql:latest",
			wantErr: true,
			expectedErrContains: []string{
				"Invalid OCI reference",
				"br:",
			},
		},
		{
			name:    "uppercase at start of repository",
			target:  "localhost:5000/MyRegistry/data:latest",
			wantErr: true,
			expectedErrContains: []string{
				"Invalid OCI reference",
				"br:",
			},
		},
		{
			name:    "invalid tag starting with hyphen",
			target:  "localhost:5000/myregistry/data:-invalid",
			wantErr: true,
			expectedErrContains: []string{
				"Invalid OCI reference",
				"br:",
			},
		},
		{
			name:    "missing repository",
			target:  "localhost:5000",
			wantErr: true,
			expectedErrContains: []string{
				"Invalid OCI reference",
				"br:",
			},
		},
		{
			name:    "valid lowercase repository",
			target:  "localhost:5000/myregistry/data/mysqldatabases/kubernetes/kubernetesmysql:latest",
			wantErr: false,
		},
		{
			name:    "valid with hyphens and underscores",
			target:  "localhost:5000/my-registry/my_data:v1.0.0",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				Target: tt.target,
			}
			_, err := r.extractDestination()
			if (err != nil) != tt.wantErr {
				t.Errorf("Runner.extractDestination() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.wantErr {
				errStr := err.Error()
				for _, expectedStr := range tt.expectedErrContains {
					if !strings.Contains(errStr, expectedStr) {
						t.Errorf("Runner.extractDestination() error = %q, expected to contain %q", errStr, expectedStr)
					}
				}
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
			got, err := r.prepareDestination()
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

func getError(registerUrl string, statusCode int) *errcode.ErrorResponse {

	err := &errcode.ErrorResponse{
		URL:        &url.URL{Host: registerUrl},
		StatusCode: statusCode,
	}

	return err
}

func TestHandleErrorResponse(t *testing.T) {

	httpErrA := getError("myregistry.azurecr.io", http.StatusUnauthorized)
	httpErrB := getError("otherregistry.gcr.io", http.StatusUnauthorized)
	httpErrC := getError("myregistry.azurecr.io", http.StatusForbidden)
	httpErrD := getError("myregistry.azurecr.io", http.StatusNotFound)
	httpErrE := getError("myregistry.azurecr.io", http.StatusInternalServerError)

	testCases := []struct {
		httpErr       *errcode.ErrorResponse
		message       string
		expectedError string
	}{
		{httpErrA, "Unauthorized - Includes Azure login info", fmt.Sprintf("Unauthorized: Please login to %q\nFor more details visit: https://learn.microsoft.com/en-us/azure/container-registry/container-registry-authentication?tabs=azure-cli Cause: \"//myregistry.azurecr.io\": response status code 401: Unauthorized.", httpErrA.URL.Host)},
		{httpErrB, "Standard unauthorized message", fmt.Sprintf("Unauthorized: Please login to %q Cause: %q: response status code 401: Unauthorized.", httpErrB.URL.Host, "//"+httpErrB.URL.Host)},
		{httpErrC, "Forbidden error", fmt.Sprintf("Forbidden: You don't have permission to push to %q Cause: %q: response status code 403: Forbidden.", httpErrC.URL.Host, "//"+httpErrC.URL.Host)},
		{httpErrD, "Not found error", fmt.Sprintf("Not Found: Unable to find registry %q Cause: %q: response status code 404: Not Found.", httpErrD.URL.Host, "//"+httpErrD.URL.Host)},
		{httpErrE, "Internal server error", fmt.Sprintf("Something went wrong Cause: %q: response status code 500: Internal Server Error.", "//"+httpErrE.URL.Host)},
	}

	for _, tc := range testCases {
		t.Run(tc.message, func(t *testing.T) {
			httpErr := &errcode.ErrorResponse{
				Method:     tc.httpErr.Method,
				URL:        tc.httpErr.URL,
				StatusCode: tc.httpErr.StatusCode,
			}

			result := handleErrorResponse(httpErr, tc.message)
			expected := fmt.Sprintf("%s\n%s", tc.message, tc.expectedError)
			require.Equal(t, expected, result.Error())
		})
	}
}
