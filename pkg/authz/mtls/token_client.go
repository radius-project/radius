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

package mtls

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/authz"
	"k8s.io/apimachinery/pkg/util/validation"
)

type tokenTransport struct {
	base      http.RoundTripper
	directory string
	namespace string
	receivers map[string]string
}

func newTokenTransport(options authz.Options, receivers []string) (http.RoundTripper, error) {
	if options.TokenDirectory == "" || !filepath.IsAbs(options.TokenDirectory) ||
		len(validation.IsDNS1123Label(options.Namespace)) != 0 {
		return nil, errors.New("external identity requires an absolute tokenDirectory and valid Radius namespace")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	t := &tokenTransport{base: transport, directory: options.TokenDirectory, namespace: options.Namespace, receivers: map[string]string{}}
	for _, receiver := range receivers {
		if !validComponent(receiver) {
			return nil, errors.New("external identity requires known receivers")
		}
		for _, host := range []string{receiver, receiver + "." + options.Namespace, receiver + "." + options.Namespace + ".svc"} {
			t.receivers[host] = receiver
		}
	}
	return t, nil
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	receiver, ok := t.receivers[req.URL.Hostname()]
	ports := map[string]string{"ucp": "9444", "applications-rp": "5443", "dynamic-rp": "8082", "bicep-de": "6443"}
	if !ok || req.URL.Scheme != "http" || req.URL.User != nil || req.URL.Port() != ports[receiver] {
		return nil, errors.New("external identity token destination is not an allowed internal HTTP receiver")
	}
	raw, err := os.ReadFile(filepath.Join(t.directory, receiver))
	if err != nil {
		return nil, errors.New("cannot read projected receiver token")
	}
	token := strings.TrimSpace(string(raw))
	if token == "" || len(token) > 16384 || strings.ContainsAny(token, " \t\r\n") {
		return nil, errors.New("projected receiver token is empty or invalid")
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(clone)
}
