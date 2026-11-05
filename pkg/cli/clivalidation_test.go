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

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
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
			want:    "",
			wantErr: errors.New("`Applications.Test/exampleResources` is not a valid resource type name. Please specify the resource type name. ex: `containers`"),
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
			wantErr: errors.New("no fully qualified resource type provided"),
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
			wantErr: fmt.Errorf("`exampleResources` is not a valid resource type. Please specify the fully qualified resource type in format `resource-provider/resource-type` and try again"),
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

func Test_RequireFullyQualifiedResourceTypeAndName(t *testing.T) {

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
			wantErr: errors.New("no resource type or name provided"),
		},
		{
			name:    "Fully-qualified resource type and name",
			args:    []string{"Applications.Test/exampleResources", "my-example"},
			want:    []string{"Applications.Test", "exampleResources", "my-example"},
			wantErr: nil,
		},
		{
			name:    "resource type not fully qualified",
			args:    []string{"exampleResources", "my-example"},
			want:    []string{},
			wantErr: fmt.Errorf("`exampleResources` is not a valid resource type. Please specify the fully qualified resource type in format `resource-provider/resource-type` and try again"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceProviderName, resourceTypeName, resourceName, err := RequireFullyQualifiedResourceTypeAndName(tt.args)
			if len(tt.want) > 0 {
				require.Equal(t, tt.want, []string{resourceProviderName, resourceTypeName, resourceName})
			} else {
				require.Equal(t, tt.wantErr, err)
			}
		})
	}
}

func Test_ReadResourceTypeNameArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "No arguments",
			args: []string{},
			want: "",
		},
		{
			name: "Resource type provided",
			args: []string{"testResources"},
			want: "testResources",
		},
		{
			name: "Multiple arguments - returns first",
			args: []string{"testResources", "extra"},
			want: "testResources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadResourceTypeNameArgs(nil, tt.args)
			require.Equal(t, tt.want, got)
		})
	}
}

// newCmdWithOutputFlag creates a cobra command with a string flag named "output" set to the given value.
func newCmdWithOutputFlag(value string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringP("output", "o", "", "output format")
	err := cmd.Flags().Set("output", value)
	if err != nil {
		panic(err)
	}
	return cmd
}

func Test_RequireOutput(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "json is accepted",
			format: "json",
			want:   "json",
		},
		{
			name:   "table is accepted",
			format: "table",
			want:   "table",
		},
		{
			name:   "empty defaults to table",
			format: "",
			want:   output.DefaultFormat,
		},
		{
			name:   "plain-text is normalized to table",
			format: "plain-text",
			want:   "table",
		},
		{
			name:      "text is rejected",
			format:    "text",
			wantErr:   true,
			errSubstr: `unsupported output format "text", supported formats are: json, table`,
		},
		{
			name:      "unknown format is rejected",
			format:    "xml",
			wantErr:   true,
			errSubstr: `unsupported output format "xml", supported formats are: json, table`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newCmdWithOutputFlag(tt.format)
			got, err := RequireOutput(cmd)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_RequireRadiusCoreEnvironmentNameArgs(t *testing.T) {
	const (
		scope        = "/planes/radius/local/resourceGroups/test-group"
		radiusCoreID = scope + "/providers/Radius.Core/environments/prod"
		legacyID     = scope + "/providers/Applications.Core/environments/prod"
	)

	tests := []struct {
		name        string
		args        []string
		flag        string
		defaultEnv  string
		want        string
		errContains string
	}{
		{name: "argument wins over legacy default", args: []string{"dev"}, defaultEnv: legacyID, want: "dev"},
		{name: "flag wins over legacy default", flag: "dev", defaultEnv: legacyID, want: "dev"},
		{name: "Radius.Core default is used", defaultEnv: radiusCoreID, want: "prod"},
		{name: "Radius.Core default type is case-insensitive", defaultEnv: scope + "/providers/radius.core/environments/prod", want: "prod"},
		{name: "legacy default is rejected", defaultEnv: legacyID, errContains: "is not a Radius.Core/environments resource"},
		{name: "no name and no default", errContains: "no environment name provided"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().StringP("environment", "e", "", "")
			if tt.flag != "" {
				require.NoError(t, cmd.Flags().Set("environment", tt.flag))
			}

			got, err := RequireRadiusCoreEnvironmentNameArgs(cmd, tt.args, workspaces.Workspace{Environment: tt.defaultEnv})
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
