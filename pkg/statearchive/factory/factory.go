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

// Package factory configures OCI state archives from environment configuration.
package factory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/statearchive"
	archiveoci "github.com/radius-project/radius/pkg/statearchive/oci"
)

const (
	// BackendEnvVar accepts "oci" or an unset value. The git backend was removed.
	BackendEnvVar = "RADIUS_STATE_BACKEND"

	// ArchivePlainHTTPEnvVar enables HTTP for a local OCI registry.
	ArchivePlainHTTPEnvVar = "RADIUS_ARCHIVE_PLAIN_HTTP"

	// StateRegistryEnvVar configures the OCI repository used by rad startup and shutdown.
	StateRegistryEnvVar = "RADIUS_STATE_REGISTRY"

	// GraphRegistryEnvVar configures the OCI repository used by modeled graph output.
	GraphRegistryEnvVar = "RADIUS_GRAPH_REGISTRY"
)

// NewStateArchive returns the OCI archive for rad startup and rad shutdown.
// Configuration errors are deferred until Archive.Open.
func NewStateArchive(registry string) statearchive.Archive {
	return newFromEnvironment(registry, StateRegistryEnvVar)
}

// NewGraphArchive returns the OCI archive for durable modeled graph output.
// Configuration errors are deferred until Archive.Open so local graph output
// and unrelated commands do not require a registry.
func NewGraphArchive(registry string) statearchive.Archive {
	return newFromEnvironment(registry, GraphRegistryEnvVar)
}

func newFromEnvironment(registry, registryEnvVar string) statearchive.Archive {
	backend := strings.ToLower(os.Getenv(BackendEnvVar))
	switch backend {
	case "", "oci":
		if registry == "" {
			return errorArchive{err: fmt.Errorf("OCI archive repository is not configured; set %s to an OCI repository (without a tag) and authenticate to the registry", registryEnvVar)}
		}
		return newOCIArchive(registry)
	case "git":
		return errorArchive{err: fmt.Errorf("the Git state archive backend has been removed; unset %s or set it to oci, configure %s, and migrate any existing Git archive data to OCI before restoring it", BackendEnvVar, registryEnvVar)}
	default:
		return errorArchive{err: fmt.Errorf("invalid %s value %q: expected oci or an unset value", BackendEnvVar, backend)}
	}
}

func newOCIArchive(registry string) statearchive.Archive {
	return archiveoci.NewOCIArchive(archiveoci.Options{
		Repository: registry,
		PlainHTTP:  strings.EqualFold(os.Getenv(ArchivePlainHTTPEnvVar), "true"),
	})
}

type errorArchive struct {
	err error
}

func (a errorArchive) Open(context.Context, string) (statearchive.Session, error) {
	return nil, a.err
}

var _ statearchive.Archive = errorArchive{}
