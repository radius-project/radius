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

package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/analyzers"
	"github.com/radius-project/radius/pkg/discovery/catalog"
	"github.com/radius-project/radius/pkg/discovery/resourcetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverNodeJSApp(t *testing.T) {
	// Get path to testdata
	testdataPath := filepath.Join("testdata", "nodejs-app")
	if _, err := os.Stat(testdataPath); os.IsNotExist(err) {
		t.Skip("testdata/nodejs-app not found")
	}

	// Initialize catalogs
	libCatalog := catalog.NewLibraryCatalog()
	err := libCatalog.LoadFromBytes([]byte(testLibraryCatalog))
	require.NoError(t, err)

	resCatalog := resourcetypes.NewCatalog()
	err = resCatalog.LoadFromBytes([]byte(testResourceCatalog))
	require.NoError(t, err)

	// Create analyzer registry
	registry := analyzers.NewRegistry()
	jsAnalyzer := analyzers.NewJavaScriptAnalyzer(libCatalog)
	err = registry.Register(jsAnalyzer)
	require.NoError(t, err)

	// Create engine
	engine, err := discovery.NewEngineWithCatalogs(registry, libCatalog, resCatalog)
	require.NoError(t, err)

	// Run discovery
	opts := discovery.DefaultDiscoverOptions(testdataPath)
	opts.OutputPath = "" // Don't write output for test

	result, err := engine.Discover(context.Background(), opts)
	require.NoError(t, err)

	// Verify results
	assert.NotNil(t, result)
	assert.Equal(t, testdataPath, result.ProjectPath)

	// Should detect services
	assert.GreaterOrEqual(t, len(result.Services), 1, "should detect at least one service")

	// Should detect PostgreSQL and Redis dependencies
	var foundPostgres, foundRedis bool
	for _, dep := range result.Dependencies {
		switch dep.Type {
		case discovery.DependencyPostgreSQL:
			foundPostgres = true
			assert.Equal(t, "pg", dep.Library)
		case discovery.DependencyRedis:
			foundRedis = true
			assert.Equal(t, "ioredis", dep.Library)
		}
	}

	assert.True(t, foundPostgres, "should detect PostgreSQL dependency (pg)")
	assert.True(t, foundRedis, "should detect Redis dependency (ioredis)")

	// Should map to Resource Types
	assert.GreaterOrEqual(t, len(result.ResourceTypes), 2, "should map dependencies to Resource Types")
}

func TestDiscoverPythonApp(t *testing.T) {
	testdataPath := filepath.Join("testdata", "python-app")
	if _, err := os.Stat(testdataPath); os.IsNotExist(err) {
		t.Skip("testdata/python-app not found")
	}

	// Initialize catalogs
	libCatalog := catalog.NewLibraryCatalog()
	err := libCatalog.LoadFromBytes([]byte(testLibraryCatalog))
	require.NoError(t, err)

	resCatalog := resourcetypes.NewCatalog()
	err = resCatalog.LoadFromBytes([]byte(testResourceCatalog))
	require.NoError(t, err)

	// Create analyzer registry
	registry := analyzers.NewRegistry()
	pyAnalyzer := analyzers.NewPythonAnalyzer(libCatalog)
	err = registry.Register(pyAnalyzer)
	require.NoError(t, err)

	engine, err := discovery.NewEngineWithCatalogs(registry, libCatalog, resCatalog)
	require.NoError(t, err)

	opts := discovery.DefaultDiscoverOptions(testdataPath)
	opts.OutputPath = ""

	result, err := engine.Discover(context.Background(), opts)
	require.NoError(t, err)

	// Verify results
	assert.NotNil(t, result)

	// Should detect Flask framework
	var foundFlask bool
	for _, svc := range result.Services {
		if svc.Framework == "Flask" {
			foundFlask = true
			break
		}
	}
	assert.True(t, foundFlask, "should detect Flask framework")

	// Should detect psycopg2 and redis
	var foundPostgres, foundRedis bool
	for _, dep := range result.Dependencies {
		switch dep.Type {
		case discovery.DependencyPostgreSQL:
			foundPostgres = true
		case discovery.DependencyRedis:
			foundRedis = true
		}
	}

	assert.True(t, foundPostgres, "should detect PostgreSQL dependency")
	assert.True(t, foundRedis, "should detect Redis dependency")
}

func TestDiscoverGoApp(t *testing.T) {
	testdataPath := filepath.Join("testdata", "go-app")
	if _, err := os.Stat(testdataPath); os.IsNotExist(err) {
		t.Skip("testdata/go-app not found")
	}

	// Initialize catalogs
	libCatalog := catalog.NewLibraryCatalog()
	err := libCatalog.LoadFromBytes([]byte(testLibraryCatalog))
	require.NoError(t, err)

	resCatalog := resourcetypes.NewCatalog()
	err = resCatalog.LoadFromBytes([]byte(testResourceCatalog))
	require.NoError(t, err)

	// Create analyzer registry
	registry := analyzers.NewRegistry()
	goAnalyzer := analyzers.NewGoAnalyzer(libCatalog)
	err = registry.Register(goAnalyzer)
	require.NoError(t, err)

	engine, err := discovery.NewEngineWithCatalogs(registry, libCatalog, resCatalog)
	require.NoError(t, err)

	opts := discovery.DefaultDiscoverOptions(testdataPath)
	opts.OutputPath = ""

	result, err := engine.Discover(context.Background(), opts)
	require.NoError(t, err)

	// Verify results
	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, len(result.Services), 1)

	// Should detect lib/pq and go-redis
	var foundPostgres, foundRedis bool
	for _, dep := range result.Dependencies {
		switch dep.Type {
		case discovery.DependencyPostgreSQL:
			foundPostgres = true
			assert.Contains(t, dep.Library, "lib/pq")
		case discovery.DependencyRedis:
			foundRedis = true
			assert.Contains(t, dep.Library, "go-redis")
		}
	}

	assert.True(t, foundPostgres, "should detect PostgreSQL dependency (lib/pq)")
	assert.True(t, foundRedis, "should detect Redis dependency (go-redis)")
}

// Test catalogs for functional tests
const testLibraryCatalog = `
version: "1.0.0"
libraries:
  - library: pg
    language: javascript
    dependencyType: postgresql
    confidence: 0.95
    defaultPort: 5432
  - library: ioredis
    language: javascript
    dependencyType: redis
    confidence: 0.95
    defaultPort: 6379
  - library: psycopg2-binary
    language: python
    dependencyType: postgresql
    confidence: 0.95
    defaultPort: 5432
  - library: redis
    language: python
    dependencyType: redis
    confidence: 0.95
    defaultPort: 6379
  - library: flask
    language: python
    dependencyType: unknown
    confidence: 0.50
  - library: github.com/lib/pq
    language: go
    dependencyType: postgresql
    confidence: 0.95
    defaultPort: 5432
  - library: github.com/redis/go-redis/v9
    language: go
    dependencyType: redis
    confidence: 0.95
    defaultPort: 6379
`

const testResourceCatalog = `
version: "1.0.0"
resourceTypes:
  - dependencyType: postgresql
    resourceTypeName: Applications.Datastores/sqlDatabases
    apiVersion: "2023-10-01-preview"
  - dependencyType: redis
    resourceTypeName: Applications.Datastores/redisCaches
    apiVersion: "2023-10-01-preview"
`
