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

package registrytest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type fakeServerInfo struct {
	TestServer   *httptest.Server
	URL          *url.URL
	CloseServer  func()
	TestImageURL string
	ImageName    string
}

// NewFakeRegistryServer creates a fake registry server that serves a single blob and index.
func NewFakeRegistryServer(t *testing.T) fakeServerInfo {
	blob := []byte(`{
	"parameters": {
		"documentdbName": {
			"type": "string"
		},
		"location": {
			"defaultValue": "[resourceGroup().location]",
			"type": "string"
		}
	}
}`)

	blobDesc := ocispec.Descriptor{
		MediaType: "recipe",
		Digest:    digest.FromBytes(blob),
		Size:      int64(len(blob)),
	}

	index := []byte(`{
	"layers": [
		{
			"digest": "` + blobDesc.Digest.String() + `"
		}
	]
}`)

	indexDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(index),
		Size:      int64(len(index)),
	}

	r := chi.NewRouter()
	r.Route("/v2/test", func(r chi.Router) {
		r.Head("/manifests/{ref}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", indexDesc.MediaType)
			w.Header().Set("Docker-Content-Digest", indexDesc.Digest.String())
			w.Header().Set("Content-Length", strconv.Itoa(int(indexDesc.Size)))
			w.WriteHeader(http.StatusOK)
		})

		r.Get("/manifests/"+indexDesc.Digest.String(), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", indexDesc.MediaType)
			w.Header().Set("Docker-Content-Digest", indexDesc.Digest.String())
			if _, err := w.Write(index); err != nil {
				t.Errorf("failed to write %q: %v", r.URL, err)
			}
		})

		r.Head("/blobs/{digest}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", blobDesc.MediaType)
			w.Header().Set("Docker-Content-Digest", blobDesc.Digest.String())
			w.Header().Set("Content-Length", strconv.Itoa(int(blobDesc.Size)))
			w.WriteHeader(http.StatusOK)
		})

		r.Get("/blobs/"+blobDesc.Digest.String(), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", blobDesc.Digest.String())
			if _, err := w.Write(blob); err != nil {
				t.Errorf("failed to write %q: %v", r.URL, err)
			}
		})
	})

	ts := httptest.NewTLSServer(r)

	url, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}

	return fakeServerInfo{
		TestServer:   ts,
		URL:          url,
		CloseServer:  ts.Close,
		TestImageURL: ts.URL + "/test:latest",
		ImageName:    "test:latest",
	}
}
