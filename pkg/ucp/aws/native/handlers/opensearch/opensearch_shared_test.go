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

package opensearch

import (
	"testing"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/require"
)

// Test_StringList checks that a list with a value that isn't a string is never read as a shorter list.
func Test_StringList(t *testing.T) {
	list, ok := stringList(map[string]any{"Ids": []any{"a", "b"}}, "Ids")
	require.True(t, ok)
	require.Equal(t, []string{"a", "b"}, list)

	list, ok = stringList(map[string]any{}, "Ids")
	require.True(t, ok)
	require.Nil(t, list)

	for _, value := range []any{[]any{"a", 5}, "a"} {
		_, ok = stringList(map[string]any{"Ids": value}, "Ids")
		require.False(t, ok, value)
	}
}

func Test_EC2InRegion(t *testing.T) {
	opts := awsec2.Options{}
	ec2InRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
