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

package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type transportFunc struct {
	Func func(req *http.Request) (*http.Response, error)
}

func (t *transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.Func(req)
}

// These tests test the mechanics of the proxy builder, and verify that the configurations
// we build work correctly.
//
// We're not testing the actual request proxying functionality since that's part of the Go std library.
func Test_ReverseProxyBuilder_Build(t *testing.T) {
	downstream, err := url.Parse("http://localhost")
	require.NoError(t, err)

	t.Run("empty", func(t *testing.T) {
		builder := &ReverseProxyBuilder{Downstream: downstream}
		proxy := builder.Build()

		require.IsType(t, &httputil.ReverseProxy{}, proxy)
		real := proxy.(*httputil.ReverseProxy)

		t.Run("correctly created", func(t *testing.T) {
			assert.Nil(t, real.Transport)

			// No good way to assert the contents of these as they are funcs.
			assert.NotNil(t, real.Director)
			assert.NotNil(t, real.ModifyResponse)
			assert.NotNil(t, real.ErrorHandler)
		})

		t.Run("successful request", func(t *testing.T) {
			real.Transport = &transportFunc{
				Func: func(req *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
				},
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(testcontext.New(t))
			w := httptest.NewRecorder()
			real.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("connection error", func(t *testing.T) {
			real.Transport = &transportFunc{
				Func: func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("connection error")
				},
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(testcontext.New(t))
			w := httptest.NewRecorder()
			real.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadGateway, w.Code)
		})
	})

	t.Run("with logging", func(t *testing.T) {
		builder := &ReverseProxyBuilder{Downstream: downstream, EnableLogging: true}
		proxy := builder.Build()

		require.IsType(t, &httputil.ReverseProxy{}, proxy)
		real := proxy.(*httputil.ReverseProxy)

		t.Run("correctly created", func(t *testing.T) {
			assert.Nil(t, real.Transport)

			// No good way to assert the contents of these as they are funcs.
			assert.NotNil(t, real.Director)
			assert.NotNil(t, real.ModifyResponse)
			assert.NotNil(t, real.ErrorHandler)
		})

		t.Run("successful request", func(t *testing.T) {
			real.Transport = &transportFunc{
				Func: func(req *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
				},
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(testcontext.New(t))
			w := httptest.NewRecorder()
			real.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("connection error", func(t *testing.T) {
			real.Transport = &transportFunc{
				Func: func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("connection error")
				},
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(testcontext.New(t))
			w := httptest.NewRecorder()
			real.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadGateway, w.Code)
		})
	})

	t.Run("with custom", func(t *testing.T) {
		builder := &ReverseProxyBuilder{Downstream: downstream}

		builder.Directors = append(builder.Directors, func(req *http.Request) {
			value := append(req.Header["Director"], "A")
			req.Header["Director"] = value
		})
		builder.Directors = append(builder.Directors, func(req *http.Request) {
			value := append(req.Header["Director"], "B")
			req.Header["Director"] = value
		})
		builder.Directors = append(builder.Directors, func(req *http.Request) {
			value := append(req.Header["Director"], "C")
			req.Header["Director"] = value
		})

		builder.Responders = append(builder.Responders, func(resp *http.Response) error {
			value := append(resp.Header["Responder"], "A")
			resp.Header["Responder"] = value
			return nil
		})
		builder.Responders = append(builder.Responders, func(resp *http.Response) error {
			value := append(resp.Header["Responder"], "B")
			resp.Header["Responder"] = value
			return nil
		})
		builder.Responders = append(builder.Responders, func(resp *http.Response) error {
			value := append(resp.Header["Responder"], "C")
			resp.Header["Responder"] = value
			return nil
		})

		builder.Transport = &transportFunc{}
		builder.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
			w.WriteHeader(http.StatusTeapot)
		}

		proxy := builder.Build()

		require.IsType(t, &httputil.ReverseProxy{}, proxy)
		real := proxy.(*httputil.ReverseProxy)

		t.Run("correctly created", func(t *testing.T) {
			assert.NotNil(t, real.Transport)
			assert.IsType(t, &transportFunc{}, real.Transport)

			// No good way to assert the contents of these as they are funcs.
			assert.NotNil(t, real.Director)
			assert.NotNil(t, real.ModifyResponse)
			assert.NotNil(t, real.ErrorHandler)
		})

		t.Run("successful request", func(t *testing.T) {
			real.Transport = &transportFunc{
				Func: func(req *http.Request) (*http.Response, error) {
					response := &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Director": req.Header["Director"]},
						Body:       http.NoBody,
						Request:    req,
					}

					return response, nil
				},
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(testcontext.New(t))
			w := httptest.NewRecorder()
			real.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, []string{"A", "B", "C"}, w.Header()["Director"])
			assert.Equal(t, []string{"C", "B", "A"}, w.Header()["Responder"])
		})

		t.Run("connection error", func(t *testing.T) {
			real.Transport = &transportFunc{
				Func: func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("connection error")
				},
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(testcontext.New(t))
			w := httptest.NewRecorder()
			real.ServeHTTP(w, req)

			assert.Equal(t, http.StatusTeapot, w.Code)
		})
	})
}
