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
