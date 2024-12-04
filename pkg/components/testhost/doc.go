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

// Package testhost provides a host for running any Radius control-plane component
// as an in-memory server for testing purposes.
//
// This package should be wrapped in a test package specific to the component under test.
// The wrapping design allows for component-specific depenendendencies to be defined without
// polluting the shared code.
package testhost
