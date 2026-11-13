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

package authz

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

// WriteDenied writes err to w. A *DeniedError becomes an ARM error response with the
// status for its code. Any other error becomes a plain 403.
func WriteDenied(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	var denied *DeniedError
	if errors.As(err, &denied) {
		if resp, rerr := denied.Response(); rerr == nil {
			if aerr := resp.Apply(ctx, w, r); aerr == nil {
				return
			}
		}
	}
	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

var (
	processMu      sync.RWMutex
	processOptions Options
)

// SetProcessOptions records the authorization options of this process. Services call it
// once at startup so that code without direct access to configuration, such as shared
// middleware and the async worker, can read the mode.
func SetProcessOptions(options Options) {
	processMu.Lock()
	defer processMu.Unlock()
	processOptions = options
}

// ProcessOptions returns the options set by SetProcessOptions.
func ProcessOptions() Options {
	processMu.RLock()
	defer processMu.RUnlock()
	return processOptions
}

// ProcessMode returns the effective mode set by SetProcessOptions.
func ProcessMode() Mode {
	return ProcessOptions().EffectiveMode()
}
