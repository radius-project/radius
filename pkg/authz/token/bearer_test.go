/*
Copyright 2026 The Radius Authors.

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

package token

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		want    string
		wantErr bool
	}{
		{name: "bearer", headers: []string{"Bearer abc.def.ghi"}, want: "abc.def.ghi"},
		{name: "lowercase scheme", headers: []string{"bearer abc"}, want: "abc"},
		{name: "missing", wantErr: true},
		{name: "repeated", headers: []string{"Bearer a", "Bearer b"}, wantErr: true},
		{name: "basic scheme", headers: []string{"Basic dXNlcjpwYXNz"}, wantErr: true},
		{name: "no token", headers: []string{"Bearer "}, wantErr: true},
		{name: "scheme only", headers: []string{"Bearer"}, wantErr: true},
		{name: "extra space", headers: []string{"Bearer  abc"}, wantErr: true},
		{name: "two tokens", headers: []string{"Bearer abc def"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, h := range tt.headers {
				r.Header.Add("Authorization", h)
			}
			got, err := BearerToken(r)
			if tt.wantErr {
				requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
