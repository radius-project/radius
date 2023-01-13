// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sdk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewDirectConnection_Valid(t *testing.T) {
	endpoint := "http://some.endpoint.com/cool-path"

	connection, err := NewDirectConnection(endpoint)
	require.NoError(t, err)

	require.IsType(t, &directConnection{}, connection)
	require.Equal(t, endpoint, connection.(*directConnection).endpoint)

	require.Equal(t, http.DefaultClient, connection.Client())
	require.Equal(t, endpoint, connection.Endpoint())
}

func Test_NewDirectConnection_InvalidUrl(t *testing.T) {
	// It's geniunely kinda hard to make Go's URL parser reject something :-|
	endpoint := ":"

	connection, err := NewDirectConnection(endpoint)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse endpoint")
	require.Nil(t, connection)
}

func Test_NewDirectConnection_InvalidWithoutScheme(t *testing.T) {
	// We require an absolute URL. Since Go's URL parser is so permissive, a lot
	// of cases will end up here.
	endpoint := "/just/a/path"

	connection, err := NewDirectConnection(endpoint)
	require.Error(t, err)
	require.Contains(t, err.Error(), "the endpoint must use the http or https scheme")
	require.Nil(t, connection)
}
