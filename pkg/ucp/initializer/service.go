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

package initializer

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	ucpoptions "github.com/radius-project/radius/pkg/ucp/hostoptions"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Service implements the hosting.Service interface for registering manifests.
type Service struct {
	ucpConnection sdk.Connection
	options       ucpoptions.UCPConfig
}

var _ hosting.Service = (*Service)(nil)

// NewService creates a server to register manifests.
func NewService(connection sdk.Connection, options ucpoptions.UCPConfig) *Service {
	return &Service{
		ucpConnection: connection,
		options:       options,
	}
}

// Name gets this service name.
func (s *Service) Name() string {
	return "initializer"
}

func waitForServer(ctx context.Context, host, port string, retryInterval time.Duration, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("connection attempts canceled or timed out: %w", ctx.Err())
		default:
			conn, err := net.DialTimeout("tcp", address, retryInterval)
			if err == nil {
				conn.Close()
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("failed to connect to %s after %v: %w", address, timeout, err)
			}

			time.Sleep(retryInterval)
		}
	}
}

func (w *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if w.ucpConnection == nil || w.ucpConnection.Endpoint() == "" {
		return fmt.Errorf("connection to UCP is not set")
	}

	// Parse the endpoint URL and extract host and port
	parsedURL, err := url.Parse(w.ucpConnection.Endpoint())
	if err != nil {
		return fmt.Errorf("failed to parse endpoint URL: %w", err)
	}

	hostName, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return fmt.Errorf("failed to split host and port: %w", err)
	}
	logger.Info("Parsed Host and Port", "host", hostName, "port", port)

	// Attempt to connect to the server
	err = waitForServer(ctx, hostName, port, 500*time.Millisecond, 5*time.Second)
	if err != nil {
		logger.Error(err, "Server is not available for manifest registration")
		return nil
	}

	// Server is up, proceed to register manifests
	manifestDir := w.options.Initialization.ManifestDirectory
	if manifestDir == "" {
		logger.Info("No manifest directory specified")
		return nil
	}

	if _, err := os.Stat(manifestDir); os.IsNotExist(err) {
		return fmt.Errorf("manifest directory does not exist: %w", err)
	} else if err != nil {
		return fmt.Errorf("error checking manifest directory: %w", err)
	}

	clientOptions := sdk.NewClientOptions(w.ucpConnection)

	clientFactory, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		logger.Error(err, "Failed to create client factory")
		return nil
	}

	// Proceed with registering manifests
	if err := manifest.RegisterDirectory(ctx, clientFactory, "local", manifestDir, nil); err != nil {
		return fmt.Errorf("error registering manifests : %w", err)
	}

	logger.Info("Successfully registered manifests", "directory", manifestDir)

	return nil
}
