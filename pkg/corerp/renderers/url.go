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

package renderers

import (
	"net"
	"net/url"
)

func IsURL(input string) bool {
	_, err := url.ParseRequestURI(input)

	// if first character is a slash, it's not a URL. It's a path.
	if input == "" || err != nil || input[0] == '/' {
		return false
	}
	return true
}

func ParseURL(sourceURL string) (scheme, hostname, port string, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", "", err
	}

	scheme = u.Scheme
	host := u.Host

	hostname, port, err = net.SplitHostPort(host)
	if err != nil {
		return "", "", "", err
	}

	return scheme, hostname, port, nil
}