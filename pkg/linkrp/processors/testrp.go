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

package processors

import (
	"net"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

// Server is a test resource provider implementation. This will create a server on a local port
// that is scoped to a single test. Use Start to create and start a new server.
type Server struct {
	// Handler is the http.Handler implementation for the server. This can be set or updated at any time, but must be
	// set before processing requests.
	Handler http.HandlerFunc

	t        *testing.T
	listener net.Listener
	inner    *httptest.Server
}

// Address returns the server's address as a string.
func (s *Server) Address() string {
	return s.listener.Addr().String()
}

// ServeHTTP is the http.Handler implementation for the server. Test should not need to call this function.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Handler == nil {
		s.t.Fatal("Handler not set")
		return
	}

	// Make it easy to debug panics
	defer func() {
		if r := recover(); r != nil {
			s.t.Fatalf("panic inside handler: \n\n%v", string(debug.Stack()))
		}
	}()

	s.Handler(w, r)
}

// Close stops the server. This will be registered as a cleanup function with the test when Start is used to start the server.
func (s *Server) Close() {
	if s.listener != nil {
		err := s.listener.Close()
		if err != nil {
			s.t.Logf("Error closing listener: %v", err)
		}
	}

	if s.inner != nil {
		s.inner.Close()
	}
}

// Starts and returns a new test resource provider implementation. The caller is responsible for
// assigning the Handler field. The server will be stopped automatically when the test completes.
func Start(t *testing.T) *Server {
	server := &Server{t: t}
	t.Cleanup(server.Close)

	// Create a listener on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:")
	require.NoError(t, err)
	server.listener = listener

	server.inner = httptest.NewUnstartedServer(server)
	server.inner.Listener = listener
	server.inner.Start()
	return server
}
