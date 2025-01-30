/*
Copyright 2024 The Radius Authors.

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

package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_RequireResourceType(t *testing.T) {

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr error
	}{
		{
			name:    "No arguments",
			args:    []string{},
			want:    "",
			wantErr: errors.New("no resource type provided"),
		},
		{
			name:    "Fully-qualified resource type",
			args:    []string{"Applications.Test/exampleResources"},
			want:    "Applications.Test/exampleResources",
			wantErr: nil,
		},
		{
			name:    "resource type not fully qualified",
			args:    []string{"exampleResources"},
			want:    "",
			wantErr: fmt.Errorf("'exampleResources' is not a valid resource type. Please specify the fully qualified resource type in format `resource-provider/resource-type` and try again"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequireResourceType(tt.args)
			if len(tt.want) > 0 {
				require.Equal(t, tt.want, got)
			} else {
				require.Equal(t, tt.wantErr, err)
			}
		})
	}
}

func Test_RequireFullyQualifiedResourceType(t *testing.T) {

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr error
	}{
		{
			name:    "No arguments",
			args:    []string{},
			want:    []string{},
			wantErr: errors.New("no resource type provided"),
		},
		{
			name:    "Fully-qualified resource type",
			args:    []string{"Applications.Test/exampleResources"},
			want:    []string{"Applications.Test", "exampleResources"},
			wantErr: nil,
		},
		{
			name:    "resource type not fully qualified",
			args:    []string{"exampleResources"},
			want:    []string{},
			wantErr: fmt.Errorf("'exampleResources' is not a valid resource type. Please specify the fully qualified resource type in format `resource-provider/resource-type` and try again"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceProviderName, resourceTypeName, err := RequireFullyQualifiedResourceType(tt.args)
			if len(tt.want) > 0 {
				require.Equal(t, tt.want, []string{resourceProviderName, resourceTypeName})
			} else {
				require.Equal(t, tt.wantErr, err)
			}
		})
	}
}
