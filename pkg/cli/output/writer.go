// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import "io"

//go:generate mockgen -destination=./mock_writer.go -package=output -self_package github.com/project-radius/radius/pkg/cli/output github.com/project-radius/radius/pkg/cli/output Interface

// interface is a mockable interface for writing cli output
type Interface interface {
	Write(format string, obj interface{}, options FormatterOptions) error
}

type OutputWriter struct{
	Writer io.Writer
}

func (o *OutputWriter) Write(format string, obj interface{}, options FormatterOptions) error{
	return Write(format, obj, o.Writer, options)
}

func Write(format string, obj interface{}, writer io.Writer, options FormatterOptions) error {
	formatter, err := NewFormatter(format)
	if err != nil {
		return err
	}

	err = formatter.Format(obj, writer, options)
	if err != nil {
		return err
	}

	return nil
}
