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

// Package factory selects a state archive implementation from environment configuration.
package factory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/statearchive"
	archivegit "github.com/radius-project/radius/pkg/statearchive/git"
	archiveoci "github.com/radius-project/radius/pkg/statearchive/oci"
)

const (
	// BackendEnvVar chooses the archive implementation.
	BackendEnvVar = "RADIUS_STATE_BACKEND"

	// ArchivePlainHTTPEnvVar enables HTTP for a local OCI registry.
	ArchivePlainHTTPEnvVar = "RADIUS_ARCHIVE_PLAIN_HTTP"

	// StateRegistryEnvVar configures the OCI repository used by rad startup and shutdown.
	StateRegistryEnvVar = "RADIUS_STATE_REGISTRY"

	// GraphRegistryEnvVar configures the OCI repository used by modeled graph output.
	GraphRegistryEnvVar = "RADIUS_GRAPH_REGISTRY"
)

// NewFromEnvironment returns the configured archive. Git is the default. OCI
// is selected when registry is set or BackendEnvVar is explicitly set to "oci".
func NewFromEnvironment(registry string) statearchive.Archive {
	backend := strings.ToLower(os.Getenv(BackendEnvVar))
	switch backend {
	case "", "git":
		if backend == "" && registry != "" {
			return newOCIArchive(registry)
		}
		return archivegit.NewGitArchive()
	case "oci":
		return newOCIArchive(registry)
	default:
		return errorArchive{err: fmt.Errorf("invalid %s value %q: expected git or oci", BackendEnvVar, backend)}
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
