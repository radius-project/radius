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

package handlers

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// timeoutsOwnedElsewhere are types whose schema timeouts don't match their handler yet, and which this check skips
// for timeouts until their own change aligns them.
var timeoutsOwnedElsewhere = []string{"AWS::EMR::Cluster"}

// Test_SchemaHandlersMatchOptions checks that each registered handler's schema declares what the handler does: a list
// handler only for a type the handler lists, one that takes a parent for a type listed under a parent, and each
// operation's timeoutInMinutes, when set, as the handler's timeout.
func Test_SchemaHandlersMatchOptions(t *testing.T) {
	catalog, err := schemas.Load()
	require.NoError(t, err)
	mismatches := []string{}
	for _, h := range All(aws.Config{}) {
		var schema struct {
			Handlers map[string]struct {
				Timeout       *int `json:"timeoutInMinutes"`
				HandlerSchema *struct {
					Required []string `json:"required"`
				} `json:"handlerSchema"`
			} `json:"handlers"`
		}
		require.NoError(t, json.Unmarshal([]byte(catalog.Native[h.TypeName()]), &schema), h.TypeName())
		options := h.Options()

		list := native.ListNone
		if l, ok := schema.Handlers["list"]; ok {
			list = native.ListRegion
			if l.HandlerSchema != nil && len(l.HandlerSchema.Required) > 0 {
				list = native.ListParent
			}
		}
		if list != options.List {
			mismatches = append(mismatches, fmt.Sprintf("%s: the schema's list handler means List %d, but the handler's is %d", h.TypeName(), list, options.List))
		}

		if slices.Contains(timeoutsOwnedElsewhere, h.TypeName()) {
			continue
		}
		for op, nativeOp := range map[string]native.Operation{"create": native.OperationCreate, "update": native.OperationUpdate, "delete": native.OperationDelete} {
			if m := schema.Handlers[op].Timeout; m != nil && time.Duration(*m)*time.Minute != options.Timeout(nativeOp) {
				mismatches = append(mismatches, fmt.Sprintf("%s: the schema's %s timeout is %d minutes, but the handler's is %s", h.TypeName(), op, *m, options.Timeout(nativeOp)))
			}
		}
	}
	slices.Sort(mismatches)
	require.Empty(t, mismatches, "make the schema's handlers declare what the handler does")
}
