/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package prompt

var _ error = (*ErrUnsupportedModel)(nil)

// ErrUnsupportedModel represents error when invalid bubble tea model is used for operations.
type ErrUnsupportedModel struct {
	Message string
}

// ErrUnsupportedModel returns the error message.
func (e *ErrUnsupportedModel) Error() string {
	return "invalid model for prompt operation"
}

// Is checks if the error provided is of type ErrUnsupportedModel.
func (e *ErrUnsupportedModel) Is(target error) bool {
	t, ok := target.(*ErrUnsupportedModel)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}
