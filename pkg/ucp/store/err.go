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

package store

var _ error = (*ErrInvalid)(nil)

type ErrInvalid struct {
	Message string
}

func (e *ErrInvalid) Error() string {
	return e.Message
}

func (e *ErrInvalid) Is(target error) bool {
	t, ok := target.(*ErrInvalid)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}

type ErrNotFound struct {
}

func (e *ErrNotFound) Error() string {
	return "the resource was not found"
}

func (e *ErrNotFound) Is(target error) bool {
	_, ok := target.(*ErrNotFound)
	return ok
}

var _ error = (*ErrInvalid)(nil)

type ErrConcurrency struct {
}

func (e *ErrConcurrency) Error() string {
	return "the operation failed due to a concurrency conflict"
}

func (e *ErrConcurrency) Is(target error) bool {
	_, ok := target.(*ErrConcurrency)
	return ok
}
