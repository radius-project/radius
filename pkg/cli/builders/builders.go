// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import "context"

type BuilderOptions struct {
	BaseDirectory string
}

type Builder interface {
	Build(ctx context.Context, values map[string]interface{}, options BuilderOptions) (map[string]interface{}, error)
}

func GetBuilders() map[string]Builder {
	builders := map[string]Builder{
		"container": &dockerBuilder{},
	}
	return builders
}
