// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

var _ Interface = (*MockOutput)(nil)

type MockOutput struct {
	Writes []any
}

type LogOutput struct {
	Format string
	Params []any
}

type FormattedOutput struct {
	Format  string
	Obj     any
	Options FormatterOptions
}

func (o *MockOutput) LogInfo(format string, v ...any) {
	o.Writes = append(o.Writes, LogOutput{Format: format, Params: v})
}

func (o *MockOutput) WriteFormatted(format string, obj any, options FormatterOptions) error {
	o.Writes = append(o.Writes, FormattedOutput{Format: format, Obj: obj, Options: options})
	return nil
}
