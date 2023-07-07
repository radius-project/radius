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

package servicecontext

import (
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

// ARMRequestCtx is the middleware to inject ARMRequestContext to the http request.
//
// # Function Explanation
//
// ARMRequestCtx is a middleware handler that adds an ARM request context to an incoming request. It takes in a pathBase
// and location string and returns a function that takes in an http.Handler. If the location string is empty, it will
// panic. Otherwise, it will attempt to create an ARM request context from the request and location strings. If it fails,
// it will return a bad request response. If successful, it will add the ARM request context to the request and pass it to
// the http.Handler.
func ARMRequestCtx(pathBase, location string) func(h http.Handler) http.Handler {
	// We normally don't like to use panics like this, but this issue is NASTY to diagnose when it happens
	// and it can regress based on changes to our configuration file format.
	//
	// The location field is used to the build the URL for an asynchronous operation. If you find that the
	// URL for an asynchronous operation has a blank segment "someText//someOtherText" then a misconfigured
	// location is probably the cause.
	if location == "" {
		panic("location is required. The location should be set in a configuration file and wired up to the middleware through the host options.")
	}

	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rpcContext, err := v1.FromARMRequest(r, pathBase, location)
			if err != nil {
				resp := rest.NewBadRequestARMResponse(v1.ErrorResponse{
					Error: v1.ErrorDetails{
						Code:    v1.CodeInvalid,
						Message: fmt.Sprintf("unexpected error: %v", err),
					},
				})

				_ = resp.Apply(r.Context(), w, r)
				return
			}

			r = r.WithContext(v1.WithARMRequestContext(r.Context(), rpcContext))
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}

// WithOperationType is the middleware to inject operation type to the http request.
func WithOperationType(operationType v1.OperationType) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			// Panic if the context doesn't include ARMRequestContext. This should never happen.
			rpcContext := v1.ARMRequestContextFromContext(ctx)
			rpcContext.OperationType = operationType
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
