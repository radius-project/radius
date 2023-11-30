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

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

func TestReadJSONBody(t *testing.T) {
	content, err := json.Marshal(map[string]string{
		"id":   "fakeID",
		"type": "fakeType",
	})
	require.NoError(t, err)

	contentTypeTests := []struct {
		contentType string
		body        []byte
		err         error
	}{
		{"application/json", content, nil},
		{"application/json; charset=utf8", content, nil},
		{"application/json;    charset=utf8", content, nil},
		{"Application/Json;    charset=utf8    ", content, nil},
		{"plain/text", content, ErrUnsupportedContentType},
	}

	for _, tc := range contentTypeTests {
		t.Run(tc.contentType, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, "http://github.com", bytes.NewBuffer(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", tc.contentType)
			// act
			parsed, err := ReadJSONBody(req)
			// assert
			if tc.err != nil {
				require.ErrorIs(t, tc.err, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, string(tc.body), string(parsed))
			}
		})
	}
}

var tag string = uuid.New().String()

func TestValidateEtag_IfMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		ifMatchEtag  string
		etagProvided string
		shouldFail   bool
	}{
		{
			"etag-provided-if-match-empty",
			"",
			"existingEtag",
			false,
		},
		{
			"etag-and-if-match-not-provided",
			"",
			"",
			false,
		},
		{
			"etag-if-match-provided-match",
			tag,
			tag,
			false,
		},
		{
			"etag-if-match-provided-no-match",
			tag,
			uuid.New().String(),
			true,
		},
		{
			"etag-not-provided-if-match-wildcard",
			"*",
			"",
			true,
		},
		{
			"etag-provided-if-match-wildcard",
			"*",
			tag,
			false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			armRequestContext := v1.ARMRequestContextFromContext(
				v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						IfMatch: tt.ifMatchEtag,
					}))
			result := ValidateETag(*armRequestContext, tt.etagProvided)
			if !tt.shouldFail {
				require.Nil(t, result)
				require.NoError(t, result)
			} else {
				require.NotNil(t, result)
				require.Error(t, result)
			}
		})
	}
}

func TestValidateEtag_IfNoneMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		ifNoneMatchEtag string
		etagProvided    string
		shouldFail      bool
	}{
		{
			"etag-and-if-none-match-empty",
			"",
			"",
			false,
		},
		{
			"etag-provided-if-none-match-empty",
			"",
			tag,
			false,
		},
		{
			"etag-empty-if-none-match-wildcard",
			"*",
			"",
			false,
		},
		{
			"etag-provided-if-none-match-wildcard",
			"*",
			tag,
			true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			armRequestContext := v1.ARMRequestContextFromContext(
				v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						IfNoneMatch: tt.ifNoneMatchEtag,
					}))
			result := ValidateETag(*armRequestContext, tt.etagProvided)
			if !tt.shouldFail {
				require.Nil(t, result)
				require.NoError(t, result)
			} else {
				require.NotNil(t, result)
				require.Error(t, result)
			}
		})
	}
}
