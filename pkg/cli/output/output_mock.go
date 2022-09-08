// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

var _ Interface = (*MockOutput)(nil)

type MockOutput struct {
	Writes []interface{}
}

type LogOutput struct {
	Format string
	Params []interface{}
}

type FormattedOutput struct {
	Format  string
	Obj     interface{}
	Options FormatterOptions
}

func (o *MockOutput) LogInfo(format string, v ...interface{}) {
	o.Writes = append(o.Writes, LogOutput{Format: format, Params: v})
}

func (o *MockOutput) WriteFormatted(format string, obj interface{}, options FormatterOptions) error {
	o.Writes = append(o.Writes, FormattedOutput{Format: format, Obj: obj, Options: options})
	return nil
}
