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

// NewStateArchive returns the archive for rad startup and rad shutdown. OCI is
// the default: when BackendEnvVar is unset, OCI is selected even without a
// registry, so a missing RADIUS_STATE_REGISTRY surfaces as a configuration
// error from Archive.Open rather than silently falling back to git. Set
// BackendEnvVar to "git" to opt into the git backend.
func NewStateArchive(registry string) statearchive.Archive {
	return newFromEnvironment(registry, true)
}

// NewGraphArchive returns the archive for modeled graph output. OCI is selected
// when a registry is configured or BackendEnvVar is "oci"; otherwise git is used
// so existing GitHub Actions workflows keep working without any configuration.
// Set BackendEnvVar to "git" to force the git backend.
func NewGraphArchive(registry string) statearchive.Archive {
	return newFromEnvironment(registry, false)
}

// newFromEnvironment selects the archive implementation. BackendEnvVar overrides
// the default: "git" always selects git and "oci" always selects OCI. When it is
// unset, OCI is selected if a registry is configured; when no registry is set,
// ociDefaultWhenUnset decides between OCI (state commands, so the missing
// registry is reported) and git (graph output, which keeps a zero-config
// fallback).
func newFromEnvironment(registry string, ociDefaultWhenUnset bool) statearchive.Archive {
	backend := strings.ToLower(os.Getenv(BackendEnvVar))
	switch backend {
	case "":
		if registry != "" || ociDefaultWhenUnset {
			return newOCIArchive(registry)
		}
		return archivegit.NewGitArchive()
	case "git":
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
