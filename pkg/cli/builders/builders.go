// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import (
	"context"
	"path"
)

type BuilderOptions struct {
	BaseDirectory   string
	PreferContainer bool
}

type Builder interface {
	Build(ctx context.Context, values interface{}, options BuilderOptions) (map[string]interface{}, error)
}

func GetBuilders() map[string]Builder {
	builders := map[string]Builder{
		"container": &dockerBuilder{},
		"npm":       &npmBuilder{},
	}
	return builders
}

func normalize(base string, p string) string {
	if path.IsAbs(p) {
		return p
	}

	return path.Join(base, p)
}
