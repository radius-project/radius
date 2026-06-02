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

package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ProviderFromID(t *testing.T) {
	t.Run("parse valid resource ID", func(t *testing.T) {
		require.Equal(t, "aws", ProviderFromID(awsMemoryDBResourceID))
		require.Equal(t, "azure", ProviderFromID(azureRedisCacheResourceID))
	})

	t.Run("parse invalid resource ID", func(t *testing.T) {
		require.Equal(t, "", ProviderFromID("\ndkdkfkdfs\t"))
	})
}
