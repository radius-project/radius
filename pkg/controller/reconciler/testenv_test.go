/*
Copyright 2026 The Radius Authors.

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

package reconciler

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// These live outside main_test.go because envtest does not build on Windows. On Windows
// TestMain is excluded, config stays nil, and envtest-backed tests skip.

// config holds a reference to the rest config for the test environment.
var config *rest.Config

// scheme holds a reference to the scheme for the test environment.
var scheme *runtime.Scheme

func SkipWithoutEnvironment(t *testing.T) {
	if config == nil {
		t.Skip("Skipping test because envtest could not be started. Running `make test` will run tests with the correct setting.")
		return
	}
}
