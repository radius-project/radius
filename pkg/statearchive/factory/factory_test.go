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

package factory

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/statearchive/oci"
	"github.com/stretchr/testify/require"
)

func TestNewGraphArchive_DefaultsToGitWithoutRegistry(t *testing.T) {
	t.Setenv(BackendEnvVar, "")

	archive := NewGraphArchive("")
	_, ok := archive.(*oci.OCIArchive)
	require.False(t, ok)
}

func TestNewGraphArchive_UsesOCIWhenRegistryConfigured(t *testing.T) {
	t.Setenv(BackendEnvVar, "")

	archive := NewGraphArchive("localhost:5000/radius-graph")
	require.IsType(t, &oci.OCIArchive{}, archive)
}

func TestNewGraphArchive_UsesOCIWhenExplicitlyConfigured(t *testing.T) {
	t.Setenv(BackendEnvVar, "oci")

	archive := NewGraphArchive("localhost:5000/radius-graph")
	require.IsType(t, &oci.OCIArchive{}, archive)
}

func TestNewGraphArchive_ExplicitGitWins(t *testing.T) {
	t.Setenv(BackendEnvVar, "git")

	archive := NewGraphArchive("localhost:5000/radius-graph")
	_, ok := archive.(*oci.OCIArchive)
	require.False(t, ok)
}

func TestNewGraphArchive_InvalidBackendFailsOnOpen(t *testing.T) {
	t.Setenv(BackendEnvVar, "filesystem")

	archive := NewGraphArchive("")
	_, err := archive.Open(context.Background(), "radius-graph")
	require.ErrorContains(t, err, "invalid "+BackendEnvVar)
}

func TestNewStateArchive_DefaultsToOCIWithoutRegistry(t *testing.T) {
	t.Setenv(BackendEnvVar, "")

	archive := NewStateArchive("")
	require.IsType(t, &oci.OCIArchive{}, archive)
}

func TestNewStateArchive_DefaultWithoutRegistryFailsOnOpen(t *testing.T) {
	t.Setenv(BackendEnvVar, "")
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	archive := NewStateArchive("")
	_, err := archive.Open(context.Background(), "radius-state")
	require.ErrorContains(t, err, "repository is not configured")
}

func TestNewStateArchive_UsesOCIWhenRegistryConfigured(t *testing.T) {
	t.Setenv(BackendEnvVar, "")

	archive := NewStateArchive("localhost:5000/radius-state")
	require.IsType(t, &oci.OCIArchive{}, archive)
}

func TestNewStateArchive_ExplicitOCIWithoutRegistryFailsOnOpen(t *testing.T) {
	t.Setenv(BackendEnvVar, "oci")
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	archive := NewStateArchive("")
	_, err := archive.Open(context.Background(), "radius-state")
	require.ErrorContains(t, err, "repository is not configured")
}

func TestNewStateArchive_ExplicitGitWins(t *testing.T) {
	t.Setenv(BackendEnvVar, "git")

	archive := NewStateArchive("localhost:5000/radius-state")
	_, ok := archive.(*oci.OCIArchive)
	require.False(t, ok)
}

func TestNewStateArchive_InvalidBackendFailsOnOpen(t *testing.T) {
	t.Setenv(BackendEnvVar, "filesystem")

	archive := NewStateArchive("")
	_, err := archive.Open(context.Background(), "radius-state")
	require.ErrorContains(t, err, "invalid "+BackendEnvVar)
}
