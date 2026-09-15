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
	"testing"

	"github.com/radius-project/radius/pkg/statearchive"
	"github.com/radius-project/radius/pkg/statearchive/oci"
	"github.com/stretchr/testify/require"
)

func TestArchiveConfiguration(t *testing.T) {
	for _, consumer := range []struct {
		name        string
		registryVar string
		newArchive  func(string) statearchive.Archive
	}{
		{name: "state", registryVar: StateRegistryEnvVar, newArchive: NewStateArchive},
		{name: "graph", registryVar: GraphRegistryEnvVar, newArchive: NewGraphArchive},
	} {
		t.Run(consumer.name, func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				backend   string
				registry  string
				wantError string
			}{
				{name: "default OCI", registry: "localhost:5000/archive"},
				{name: "explicit OCI", backend: "oci", registry: "localhost:5000/archive"},
				{name: "case insensitive OCI", backend: "OCI", registry: "localhost:5000/archive"},
				{name: "default missing registry", wantError: "repository is not configured"},
				{name: "explicit OCI missing registry", backend: "oci", wantError: "repository is not configured"},
				{name: "removed git without registry", backend: "git", wantError: "Git state archive backend has been removed"},
				{name: "removed git with registry", backend: "git", registry: "localhost:5000/archive", wantError: "Git state archive backend has been removed"},
				{name: "case insensitive git", backend: "GIT", wantError: "Git state archive backend has been removed"},
				{name: "unknown backend", backend: "filesystem", wantError: "invalid " + BackendEnvVar},
				{name: "unknown backend with registry", backend: "filesystem", registry: "localhost:5000/archive", wantError: "invalid " + BackendEnvVar},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Setenv(BackendEnvVar, tc.backend)
					t.Setenv("DOCKER_CONFIG", t.TempDir())
					archive := consumer.newArchive(tc.registry)
					require.NotNil(t, archive, "configuration must not prevent CLI initialization")
					if tc.wantError == "" {
						require.IsType(t, &oci.OCIArchive{}, archive)
						return
					}

					session, err := archive.Open(t.Context(), "radius-"+consumer.name)
					require.Nil(t, session)
					require.ErrorContains(t, err, tc.wantError)
					if tc.backend == "filesystem" {
						require.ErrorContains(t, err, "expected oci or an unset value")
					} else {
						require.ErrorContains(t, err, consumer.registryVar)
					}
					if tc.backend == "git" || tc.backend == "GIT" {
						require.ErrorContains(t, err, "unset "+BackendEnvVar+" or set it to oci")
						require.ErrorContains(t, err, "OCI repository")
					}
				})
			}
		})
	}
}
