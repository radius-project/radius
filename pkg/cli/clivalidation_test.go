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
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/stretchr/testify/require"
)

func Test_RequireResourceType(t *testing.T) {

	supportedTypes := []string{}

	for _, resourceType := range clients.ResourceTypesList {
		supportedType := strings.Split(resourceType, "/")[1]
		supportedTypes = append(supportedTypes, supportedType)
	}

	resourceTypesErrorString := strings.Join(supportedTypes, "\n")

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
			name:    "Supported resource type",
			args:    []string{"mongoDatabases"},
			want:    "Applications.Datastores/mongoDatabases",
			wantErr: nil,
		},
		{
			name:    "Multiple resource types",
			args:    []string{"secretStores"},
			want:    "",
			wantErr: fmt.Errorf("multiple resource types match 'secretStores'. Please specify the full resource type and try again:\n\nApplications.Dapr/secretStores\nApplications.Core/secretStores\n"),
		},
		{
			name:    "Unsupported resource type",
			args:    []string{"unsupported"},
			want:    "",
			wantErr: fmt.Errorf("'unsupported' is not a valid resource type. Available Types are: \n\n" + resourceTypesErrorString + "\n"),
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
