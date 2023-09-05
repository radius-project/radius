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
			target:  "test.azurecr.io/test/repo",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "no repo",
			target:  "test.azurecr.io",
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
		name    string
		dest    *destination
		want    *remote.Repository
		wantErr bool
	}{
		{
			name: "prepare destination",
			dest: &destination{
				host: "index.docker.io",
				repo: "repo",
				tag:  "tag",
			},
			want: &remote.Repository{
				Reference: registry.Reference{
					Registry:   "index.docker.io",
					Repository: "repo",
					Reference:  "tag",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				Destination: tt.dest,
			}
			got, err := r.prepareDestination(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Runner.prepareDestination() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			require.Equal(t, tt.want.Reference.Registry, got.Reference.Registry)
			require.Equal(t, tt.want.Reference.Repository, got.Reference.Repository)
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
				"br:test.azurecr.io/test/repo:tag",
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
				"test.azurecr.io/test/repo:tag",
			},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, tests)
}
