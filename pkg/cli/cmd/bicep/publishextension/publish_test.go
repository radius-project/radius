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

package publishextension

import (
	"testing"

	"github.com/radius-project/radius/test/radcli"
)

// NOTE: this command orchestrates other CLI commands, and so it's not very testable. This will be covered with
// functional tests.

func TestRunner_Validate(t *testing.T) {
	tests := []radcli.ValidateInput{
		{
			Name:          "Valid",
			Input:         []string{"--from-file", "testdata/valid.yaml", "--target", "./output.tgz"},
			ExpectedValid: true,
		},
		{
			Name:          "Invalid: invalid manifest",
			Input:         []string{"--from-file", "testdata/invalid.yaml", "--target", "./output.tgz"},
			ExpectedValid: false,
		},
		{
			Name:          "Invalid: missing required options",
			Input:         []string{"--from-file", "testdata/valid.yaml"},
			ExpectedValid: false,
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, tests)
}
