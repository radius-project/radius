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

package operations

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

var removalSchema = []byte(`{
	"properties": {},
	"readOnlyProperties": ["/properties/Arn"],
	"createOnlyProperties": ["/properties/BucketName", "/properties/Encryption/KmsKeyId", "/properties/Members/*/Name"]
}`)

func Test_GeneratePatchWithOptions_Removed(t *testing.T) {
	t.Parallel()

	current := map[string]any{
		"BucketName":              "b",
		"Arn":                     "arn:aws:s3:::b",
		"Tags":                    []any{map[string]any{"Key": "k", "Value": "v"}},
		"VersioningConfiguration": map[string]any{"Status": "Enabled"},
		"Logging":                 map[string]any{"Prefix": "p", "Bucket": "logs"},
		"SetByAWS":                "default",
		"Members":                 []any{map[string]any{"Name": "m", "Role": "r"}},
	}

	tests := []struct {
		name    string
		desired map[string]any
		removed []string
		want    []string
	}{
		{
			name:    "no record keeps every property the template does not set",
			desired: map[string]any{"BucketName": "b"},
			want:    nil,
		},
		{
			name:    "removed top-level object is cleared",
			desired: map[string]any{"BucketName": "b"},
			removed: []string{"VersioningConfiguration/Status"},
			want:    []string{"remove /VersioningConfiguration"},
		},
		{
			name:    "removed nested property clears only that property",
			desired: map[string]any{"BucketName": "b", "Logging": map[string]any{"Bucket": "logs"}},
			removed: []string{"Logging/Prefix"},
			want:    []string{"remove /Logging/Prefix"},
		},
		{
			name:    "removed array is cleared as one value",
			desired: map[string]any{"BucketName": "b"},
			removed: []string{"Tags"},
			want:    []string{"remove /Tags"},
		},
		{
			name:    "read-only and create-only properties are never cleared",
			desired: map[string]any{},
			removed: []string{"Arn", "BucketName"},
			want:    nil,
		},
		{
			name:    "an array holding a create-only property is never cleared",
			desired: map[string]any{"BucketName": "b"},
			removed: []string{"Members"},
			want:    nil,
		},
		{
			name:    "a property in neither the template nor the record is kept",
			desired: map[string]any{"BucketName": "b"},
			removed: []string{"NotInAWS"},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentJSON, err := json.Marshal(current)
			require.NoError(t, err)
			desiredJSON, err := json.Marshal(tt.desired)
			require.NoError(t, err)

			patch, err := GeneratePatchWithOptions(currentJSON, desiredJSON, removalSchema, PatchOptions{Removed: tt.removed})
			require.NoError(t, err)
			got := []string{}
			for _, op := range patch {
				got = append(got, op.Type+" "+op.Path)
			}
			require.ElementsMatch(t, tt.want, got)
		})
	}
}

func Test_RemovedPaths(t *testing.T) {
	t.Parallel()

	lastApplied := map[string]any{
		"A":    "1",
		"B":    map[string]any{"C": "2", "D": "3"},
		"List": []any{"x", "y"},
		"Gone": map[string]any{"E": map[string]any{"F": "4"}},
	}
	desired := map[string]any{
		"A":    "changed",
		"B":    map[string]any{"C": "2"},
		"List": []any{"x"},
	}

	require.Equal(t, []string{"B/D", "Gone/E/F"}, RemovedPaths(lastApplied, desired))
	require.Empty(t, RemovedPaths(nil, desired))
	require.Equal(t, []string{"A", "B/C", "B/D", "Gone/E/F", "List"}, RemovedPaths(lastApplied, nil))
}

func Test_RemovedCreateOnly(t *testing.T) {
	t.Parallel()

	current := map[string]any{
		"BucketName": "b",
		"Arn":        "arn",
		"Encryption": map[string]any{"KmsKeyId": "key"},
		"Other":      "x",
		"Members":    []any{map[string]any{"Name": "m"}},
	}

	got, err := RemovedCreateOnly(current, removalSchema, []string{"Other", "Encryption/KmsKeyId", "BucketName", "Arn", "Members"})
	require.NoError(t, err)
	require.Equal(t, []string{"BucketName", "Encryption/KmsKeyId", "Members"}, got)

	// AWS no longer reports it, so there is nothing to clear.
	got, err = RemovedCreateOnly(map[string]any{}, removalSchema, []string{"BucketName"})
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = RemovedCreateOnly(current, []byte("not json"), nil)
	require.Error(t, err)
}

func Test_FlattenProperties_Null(t *testing.T) {
	t.Parallel()
	require.Equal(t, map[string]any{"A": nil, "B/C": "d"}, FlattenProperties(map[string]any{"A": nil, "B": map[string]any{"C": "d"}}))
}

func Test_OverlapsAny(t *testing.T) {
	t.Parallel()
	paths := []string{"A/B", "Members/*/Name"}
	for k, want := range map[string]bool{
		"A":              true,
		"A/B":            true,
		"A/B/C":          true,
		"A/C":            false,
		"Members":        true,
		"Members/0/Name": true,
		"Members/0/Role": false,
		"Other":          false,
	} {
		require.Equal(t, want, overlapsAny(k, paths), k)
	}
}
