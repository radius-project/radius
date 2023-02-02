// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
