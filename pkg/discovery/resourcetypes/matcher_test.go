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

package resourcetypes

import (
	"testing"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalog_LoadFromBytes(t *testing.T) {
	catalogYAML := `
version: "2.0.0"
resourceTypes:
  - dependencyType: postgresql
    resourceTypeName: Radius.Data/postgreSqlDatabases
    apiVersion: "2025-08-01-preview"
    description: PostgreSQL database
  - dependencyType: redis
    resourceTypeName: Radius.Data/redisCaches
    apiVersion: "2025-08-01-preview"
`

	catalog := NewCatalog()
	err := catalog.LoadFromBytes([]byte(catalogYAML))
	require.NoError(t, err)

	assert.Equal(t, 2, catalog.Size())
}

func TestCatalog_Lookup(t *testing.T) {
	catalogYAML := `
version: "2.0.0"
resourceTypes:
  - dependencyType: postgresql
    resourceTypeName: Radius.Data/postgreSqlDatabases
    apiVersion: "2025-08-01-preview"
  - dependencyType: redis
    resourceTypeName: Radius.Data/redisCaches
    apiVersion: "2025-08-01-preview"
`

	catalog := NewCatalog()
	err := catalog.LoadFromBytes([]byte(catalogYAML))
	require.NoError(t, err)

	// Test successful lookup
	entry, found := catalog.Lookup(dtypes.DependencyPostgreSQL)
	assert.True(t, found)
	assert.Equal(t, "Radius.Data/postgreSqlDatabases", entry.ResourceTypeName)
	assert.Equal(t, "2025-08-01-preview", entry.APIVersion)

	// Test Redis lookup
	entry, found = catalog.Lookup(dtypes.DependencyRedis)
	assert.True(t, found)
	assert.Equal(t, "Radius.Data/redisCaches", entry.ResourceTypeName)

	// Test not found
	_, found = catalog.Lookup(dtypes.DependencyKafka)
	assert.False(t, found)
}

func TestCatalog_All(t *testing.T) {
	catalog := NewCatalog()
	catalog.Add(ResourceTypeEntry{
		DependencyType:   dtypes.DependencyPostgreSQL,
		ResourceTypeName: "Radius.Data/postgreSqlDatabases",
		APIVersion:       "2025-08-01-preview",
	})
	catalog.Add(ResourceTypeEntry{
		DependencyType:   dtypes.DependencyRedis,
		ResourceTypeName: "Radius.Data/redisCaches",
		APIVersion:       "2025-08-01-preview",
	})

	all := catalog.All()
	assert.Len(t, all, 2)
}

func TestResourceTypeEntry_ToResourceType(t *testing.T) {
	entry := ResourceTypeEntry{
		DependencyType:   dtypes.DependencyPostgreSQL,
		ResourceTypeName: "Radius.Data/postgreSqlDatabases",
		APIVersion:       "2025-08-01-preview",
		DefaultProperties: map[string]interface{}{
			"database": "postgres",
		},
		SchemaURL: "https://raw.githubusercontent.com/radius-project/resource-types-contrib/main/resources/Radius.Data/postgreSqlDatabases/types.json",
	}

	rt := entry.ToResourceType()

	assert.Equal(t, "Radius.Data/postgreSqlDatabases", rt.Name)
	assert.Equal(t, "2025-08-01-preview", rt.APIVersion)
	assert.Equal(t, "postgres", rt.Properties["database"])
	assert.Equal(t, "https://raw.githubusercontent.com/radius-project/resource-types-contrib/main/resources/Radius.Data/postgreSqlDatabases/types.json", rt.Schema)
}

func TestMatch(t *testing.T) {
	dep := dtypes.DetectedDependency{
		ID:         "postgres-1",
		Type:       dtypes.DependencyPostgreSQL,
		Name:       "pg",
		Library:    "pg",
		Confidence: 0.95,
	}

	entry := ResourceTypeEntry{
		DependencyType:   dtypes.DependencyPostgreSQL,
		ResourceTypeName: "Radius.Data/postgreSqlDatabases",
		APIVersion:       "2025-08-01-preview",
	}

	mapping := Match(dep, entry)

	assert.Equal(t, "postgres-1", mapping.DependencyID)
	assert.Equal(t, "Radius.Data/postgreSqlDatabases", mapping.ResourceType.Name)
	assert.Equal(t, dtypes.MatchCatalog, mapping.MatchSource)
	// Confidence should be slightly reduced
	assert.Less(t, mapping.Confidence, dep.Confidence)
}
