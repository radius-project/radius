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

func (o *OutputWriter) LogInfo(format string, v ...any) {
	fmt.Fprintf(o.Writer, format, v...)
	fmt.Fprintln(o.Writer)
}

func (o *OutputWriter) WriteFormatted(format string, obj any, options FormatterOptions) error {
	return Write(format, obj, o.Writer, options)
}

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
