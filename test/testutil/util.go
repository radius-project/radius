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

package testutil

import (
	"encoding/json"
	"os"
)

// MustGetTestData reads testdata and unmarshals it to the given type.
func MustGetTestData[T any](file string) *T {
	var data T
	err := json.Unmarshal(ReadFixture(file), &data)
	if err != nil {
		panic(err)
	}
	return &data
}

// ReadFixture reads testdata fixtures.
func ReadFixture(filename string) []byte {
	raw, err := os.ReadFile("./testdata/" + filename)
	if err != nil {
		panic(err)
	}
	return raw
}
