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

package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuery_Validate(t *testing.T) {
	tests := []struct {
		name    string
		query   Query
		wantErr bool
	}{
		{
			name:    "ResourceType is empty",
			query:   Query{ResourceType: "", RootScope: "/planes"},
			wantErr: true,
		},
		{
			name:    "RootScope is empty",
			query:   Query{ResourceType: "Applications.Core/applications", RootScope: ""},
			wantErr: true,
		},
		{
			name: "ScopeQuery with RoutingScopePrefix",
			query: Query{
				ResourceType:       "Applications.Core/applications",
				RootScope:          "/planes",
				IsScopeQuery:       true,
				RoutingScopePrefix: "/asdf",
			},
			wantErr: true,
		},
		{
			name: "Filter is invalid",
			query: Query{
				ResourceType: "Applications.Core/applications",
				RootScope:    "/planes",
				Filters:      []QueryFilter{{Field: "invalid field!", Value: "some value"}},
			},
			wantErr: true,
		},
		{
			name: "Valid",
			query: Query{
				ResourceType: "Applications.Core/applications",
				RootScope:    "/planes",
				Filters:      []QueryFilter{{Field: "location", Value: "some value"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if tt.wantErr {
				require.Error(t, err, "expected an error but got none")
			} else {
				require.NoError(t, err, "expected no error but got one")
			}
		})
	}
}

func TestQueryFilter_Validate(t *testing.T) {
	tests := []struct {
		name    string
		filter  QueryFilter
		wantErr bool
	}{
		{
			name:    "Field is empty",
			filter:  QueryFilter{Field: "", Value: "some value"},
			wantErr: true,
		},
		{
			name:    "Field is invalid (contains special characters)",
			filter:  QueryFilter{Field: "invalid[field]", Value: "some value"},
			wantErr: true,
		},
		{
			name:    "Field is valid (property)",
			filter:  QueryFilter{Field: "properties.application", Value: "some value"},
			wantErr: false,
		},
		{
			name:    "Field is valid (path)",
			filter:  QueryFilter{Field: "properties.application.some.other.thing", Value: "some value"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if tt.wantErr {
				require.Error(t, err, "expected an error but got none")
			} else {
				require.NoError(t, err, "expected no error but got one")
			}
		})
	}
}
