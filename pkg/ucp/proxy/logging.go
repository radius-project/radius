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
	"fmt"
	"net/http"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func logUpstreamRequest(r *http.Request) {
	logger := ucplog.FromContextOrDiscard(r.Context())
	logger.Info("preparing proxy request")
}

func logDownstreamRequest(r *http.Request) {
	logger := ucplog.FromContextOrDiscard(r.Context())
	logger.Info("sending proxy request to downstream")
}

func logDownstreamResponse(r *http.Response) error {
	logger := ucplog.FromContextOrDiscard(r.Request.Context())
	logger.Info(fmt.Sprintf("received proxy response HTTP status code from downstream %d", r.StatusCode))
	return nil
}

func logUpstreamResponse(r *http.Response) error {
	logger := ucplog.FromContextOrDiscard(r.Request.Context())
	logger.Info(fmt.Sprintf("sending proxy response %d to upstream", r.StatusCode))
	return nil
}

func logConnectionError(original ErrorHandlerFunc) ErrorHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		logger := ucplog.FromContextOrDiscard(r.Context())
		logger.Error(err, "connection failed to downstream")
		original(w, r, err)
	}
}
