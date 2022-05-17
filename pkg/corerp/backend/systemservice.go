// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/version"
)

// SystemService represents the service which provides the basic health status and metric server.
type SystemService struct {
	Options hostoptions.HostOptions
}

func NewSystemService(options hostoptions.HostOptions) *SystemService {
	return &SystemService{
		Options: options,
	}
}

func (s *SystemService) Name() string {
	return "system service"
}

func (s *SystemService) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/version", reportVersion)
	mux.HandleFunc("/healthz", reportVersion)

	// TODO: Add prometheus metric later.

	address := fmt.Sprintf(":%d", *s.Options.Config.WorkerServer.SystemHTTPServerPort)

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

	logger.Info("System http server listening on: '%s'...", address)
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

func reportVersion(w http.ResponseWriter, req *http.Request) {
	info := version.NewVersionInfo()

	b, err := json.MarshalIndent(&info, "", "  ")

	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	_, _ = w.Write(b)
}
