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
	"strings"
)

// BearerToken returns the token from the request's single "Authorization: Bearer <token>"
// header. The scheme is case-insensitive. A missing, repeated, or malformed header
// returns an *Error with code InvalidAuthenticationInfo.
func BearerToken(r *http.Request) (string, error) {
	values := r.Header.Values("Authorization")
	switch len(values) {
	case 0:
		return "", invalid("missing bearer token", nil)
	case 1:
	default:
		return "", invalid("more than one Authorization header", nil)
	}

	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", invalid("Authorization header is not a bearer token", nil)
	}
	// RFC 6750 section 2.1 allows one or more ASCII spaces after the scheme.
	token = strings.TrimLeft(token, " ")
	if !isB64Token(token) {
		return "", invalid("malformed bearer token", nil)
	}
	return token, nil
}

// isB64Token reports whether s matches RFC 6750's
// b64token = 1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"=".
func isB64Token(s string) bool {
	body := strings.TrimRight(s, "=")
	if body == "" {
		return false
	}
	for _, c := range []byte(body) {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '.', c == '_', c == '~', c == '+', c == '/':
		default:
			return false
		}
	}
	return true
}
