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

package step

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/radius-project/radius/test/radcli"
)

// ErrorContainsAny reports whether err - including the nested ARM error details
// that rad surfaces inside a CLIError - contains any of the given substrings.
//
// rad returns the deployment root cause inside nested ARM error
// details[].message fields, while CLIError.Error() only exposes the top-level
// code and message (for example "DeploymentFailed"). ErrorContainsAny therefore
// flattens the full ARM error response before matching so callers can detect
// deeply nested causes. It is the shared building block for transient-error
// retry predicates such as IsTransientImagePullError.
func ErrorContainsAny(err error, substrings ...string) bool {
	if err == nil {
		return false
	}

	haystack := err.Error()
	if cliErr, ok := errors.AsType[*radcli.CLIError](err); ok {
		if encoded, marshalErr := json.Marshal(cliErr.ErrorResponse); marshalErr == nil {
			haystack += " " + string(encoded)
		}
	}

	for _, s := range substrings {
		if strings.Contains(haystack, s) {
			return true
		}
	}

	return false
}
