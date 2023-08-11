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

package v20220315privatepreview

import (
	"os"
	"time"
)

// FakeResource is a fake resource type.
type FakeResource struct{}

// # Function Explanation
//
// Always returns "FakeResource" as the name.
func (f *FakeResource) ResourceTypeName() string {
	return "FakeResource"
}

// LoadTestData reads the contents of a file and returns it as a byte slice.
// It takes a single argument, testfile, which is the path to the file to be read.
// If the file cannot be read, an error is returned.
func LoadTestData(testfile string) ([]byte, error) {
	d, err := os.ReadFile(testfile)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// UnmarshalTimeString unmarshals a string representation of a time in RFC3339 format into a time.Time object.
func UnmarshalTimeString(ts string) *time.Time {
	var tt timeRFC3339
	_ = tt.UnmarshalText([]byte(ts))
	return (*time.Time)(&tt)
}
