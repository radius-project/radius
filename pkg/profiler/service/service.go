// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

// NewService of profiler package returns a new Service with the configs needed
func NewService(options HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

// Name method of profiler package returns the name of the profiler service
func (s *Service) Name() string {
	return "pprof profiler"
}

// Run method of profiler package creates a new server for exposing an endpoint to collect profiler from
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
