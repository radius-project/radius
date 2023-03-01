// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/version"
)

// SystemService represents the service which provides the basic health status and metric server.
type SystemService struct {
	options hostoptions.HostOptions
}

// NewSystemService creates SystemService instance.
func NewSystemService(options hostoptions.HostOptions) *SystemService {
	return &SystemService{
		options: options,
	}
}

func (s *SystemService) Name() string {
	return "system service"
}

func (s *SystemService) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/version", version.ReportVersionHandler)
	mux.HandleFunc("/healthz", version.ReportVersionHandler)

	// TODO: Add prometheus metric later.

	address := fmt.Sprintf(":%d", *s.options.Config.WorkerServer.Port)

	server := &http.Server{
		Addr:    address,
		Handler: mux,
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

	logger.Info(fmt.Sprintf("System service endpoint on: '%s'...", address))
	if err := server.ListenAndServe(); err == http.ErrServerClosed {
		// We expect this, safe to ignore.
		logger.Info("Server stopped...")
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Server stopped...")
	return nil
}
