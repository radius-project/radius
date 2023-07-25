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

package profilerservice

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

type Service struct {
	Options HostOptions
}

// # Function Explanation
//
// NewService of profiler package returns a new Service with the configs needed
func NewService(options HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

// # Function Explanation
//
// Name returns the name of the profiler service.
func (s *Service) Name() string {
	return "pprof profiler"
}

// # Function Explanation
//
// Run starts the profiler server and handles shutdown based on the context, returning an error if the server fails to start.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	profilerPort := strconv.Itoa(s.Options.Config.Port)
	server := &http.Server{
		Addr: ":" + profilerPort,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("profiler Server listening on localhost port: '%s'...", profilerPort))
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		// We expect this, safe to ignore.
		logger.Info("Server stopped...")
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Server stopped...")
	return nil
}
