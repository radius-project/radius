// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package logstream

import (
	"context"
	"io"
)

// Options specifies the options for streaming application logs.
type Options struct {
	// ApplicationName is the name of the application.
	ApplicationName string

	// Namespace is the kubernetes namespace of the application.
	Namespace string

	// KubeContext is the name of the kubernetes context to use for connection.
	KubeContext string

	// Out is where output will be written.
	Out io.Writer
}

//go:generate mockgen -destination=./mock_logstream.go -package=logstream -self_package github.com/project-radius/radius/pkg/cli/kubernetes/logstream github.com/project-radius/radius/pkg/cli/kubernetes/logstream Interface

// Interface is the interface type for streaming application logs.
type Interface interface {
	// Stream opens a log stream and writes the application's log to the provided writer.
	// This function will block until the context is cancelled.
	Stream(ctx context.Context, options Options) error
}
