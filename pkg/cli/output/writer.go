// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"fmt"
	"io"
)

// Interface is a mockable interface for writing cli output
type Interface interface {
	// LogInfo logs a message that is displayed to the user by default.
	LogInfo(format string, v ...any)

	// Write
	WriteFormatted(format string, obj any, options FormatterOptions) error
}

type OutputWriter struct {
	Writer io.Writer
}

// # Function Explanation
// 
//	OutputWriter.LogInfo() prints a formatted string to the Writer object, followed by a new line. It handles any errors 
//	that occur while printing.
func (o *OutputWriter) LogInfo(format string, v ...any) {
	fmt.Fprintf(o.Writer, format, v...)
	fmt.Fprintln(o.Writer)
}

// # Function Explanation
// 
//	OutputWriter.WriteFormatted() writes the given object in the specified format to the Writer, using the given 
//	FormatterOptions. It returns an error if the write fails.
func (o *OutputWriter) WriteFormatted(format string, obj any, options FormatterOptions) error {
	return Write(format, obj, o.Writer, options)
}

// # Function Explanation
// 
//	Write creates a new formatter based on the given format string, formats the given object using the formatter, and writes
//	 the result to the given writer. If any errors occur during the process, they are returned to the caller.
func Write(format string, obj any, writer io.Writer, options FormatterOptions) error {
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
