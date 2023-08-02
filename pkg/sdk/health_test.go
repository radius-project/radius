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
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_reportErrorFromResponse(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusBadGateway,
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Another-Header": []string{"some", "value"},
			},
			Body: nil,
		}

		expected := errors.New(`An unknown error was returned while testing Radius API status:
Status Code: 502
Response Headers:
  Another-Header: some
  Another-Header: value
  Content-Type: application/json
Response Body: (empty)
`)
		actual := reportErrorFromResponse(response)
		require.Equal(t, expected, actual)
	})

	t.Run("with body", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusBadGateway,
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Another-Header": []string{"some", "value"},
			},
			Body: io.NopCloser(bytes.NewBufferString(`{"message": "some error"}`)),
		}

		expected := errors.New(`An unknown error was returned while testing Radius API status:
Status Code: 502
Response Headers:
  Another-Header: some
  Another-Header: value
  Content-Type: application/json
Response Body:
{"message": "some error"}
`)
		actual := reportErrorFromResponse(response)
		require.Equal(t, expected, actual)
	})
}
