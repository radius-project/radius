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

// # Function Explanation
// 
//	MockOutput.LogInfo appends a LogOutput struct containing the format string and parameters to the Writes slice, allowing 
//	callers to track the output of the function. Error handling is implicit, as no errors are returned.
func (o *MockOutput) LogInfo(format string, v ...any) {
	o.Writes = append(o.Writes, LogOutput{Format: format, Params: v})
}

// # Function Explanation
// 
//	MockOutput.WriteFormatted appends a FormattedOutput object containing the given format, object and options to the Writes
//	 slice, and returns nil if successful. If an error occurs, it will be returned instead.
func (o *MockOutput) WriteFormatted(format string, obj any, options FormatterOptions) error {
	o.Writes = append(o.Writes, FormattedOutput{Format: format, Obj: obj, Options: options})
	return nil
}
