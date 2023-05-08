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

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_PathParser(t *testing.T) {
	repository, tag, err := parsePath("radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0")
	require.NoError(t, err)
	require.Equal(t, "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure", repository)
	require.Equal(t, "1.0", tag)
}

func Test_PathParserErr(t *testing.T) {
	repository, tag, err := parsePath("http://user:passwd@example.com/test/bar:v1")
	require.Error(t, err)
	require.Equal(t, "", repository)
	require.Equal(t, "", tag)
}
