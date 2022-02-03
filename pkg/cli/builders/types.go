// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import (
	"context"
	"path"

	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/output"
)

type Options struct {
	BaseDirectory string
	Registry      *environments.Registry
	Output        *output.Stream
	Values        interface{}
}

type Output struct {
	// Result is a value representing the build output. If provided non-nil, the Result will be
	// added as a parameter for the stage to consume.
	Result interface{}
}

type Builder interface {
	Build(ctx context.Context, options Options) (Output, error)
}

func NormalizePath(base string, p string) string {
	if path.IsAbs(p) {
		return p
	}

	return path.Join(base, p)
}

func DefaultBuilders() map[string]Builder {
	return map[string]Builder{
		"docker": &dockerBuilder{},
	}
}
