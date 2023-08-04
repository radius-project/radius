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

package sdk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func Test_NewDirectConnection_Valid(t *testing.T) {
	endpoint := "http://example.com"

	connection, err := NewDirectConnection(endpoint)
	require.NoError(t, err)

	require.IsType(t, &directConnection{}, connection)
	require.Equal(t, endpoint, connection.(*directConnection).endpoint)
	require.IsType(t, &http.Client{}, connection.Client())
	require.IsType(t, &otelhttp.Transport{}, connection.Client().Transport)
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
