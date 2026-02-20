/*
Copyright 2026 The Radius Authors.

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

package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	dbinmemory "github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	qinmem "github.com/radius-project/radius/pkg/components/queue/inmemory"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/ucp"
)

// RegisterRoutesWithHostOptions registers installer endpoints on a router using HostOptions.
// This is used by applications-rp which uses HostOptions instead of UCP Options.
func RegisterRoutesWithHostOptions(ctx context.Context, r chi.Router, options hostoptions.HostOptions, pathBase string) error {
	var (
		qClient  queue.Client
		dbClient database.Client
		err      error
	)

	// Queue client: Create a dedicated queue for the terraform installer.
	if options.Config.QueueProvider.Provider != "" {
		qOpts := options.Config.QueueProvider
		qOpts.Name = QueueName
		qp := queueprovider.New(qOpts)
		qClient, err = qp.GetClient(ctx)
		if err != nil {
			return err
		}
	} else {
		// Fallback for tests and minimal configurations
		qClient = qinmem.NewNamedQueue(QueueName)
	}

	// Database client using the shared provider when configured; fallback to in-memory for tests/local.
	if options.Config.DatabaseProvider.Provider != "" {
		dbProvider := databaseprovider.FromOptions(options.Config.DatabaseProvider)
		dbClient, err = dbProvider.GetClient(ctx)
		if err != nil {
			return err
		}
	} else {
		dbClient = dbinmemory.NewClient()
	}

	statusStore := NewStatusStore(dbClient, StatusStorageID)
	handler := &HTTPHandler{
		Queue:       qClient,
		StatusStore: statusStore,
		RootPath:    terraformPathFromHostOptions(&options),
	}

	basePath := strings.TrimSuffix(pathBase, "/") + "/installer/terraform"
	r.Route(basePath, func(route chi.Router) {
		route.Post("/install", handler.Install)
		route.Post("/uninstall", handler.Uninstall)
		route.Get("/status", handler.Status)
	})

	return nil
}

// RegisterRoutes registers installer endpoints on the UCP router.
// Deprecated: Use RegisterRoutesWithHostOptions for applications-rp.
func RegisterRoutes(ctx context.Context, r chi.Router, options *ucp.Options) error {
	var (
		qClient  queue.Client
		dbClient database.Client
		err      error
	)

	// Queue client: Create a dedicated queue for the terraform installer.
	// We need a named queue (terraform-installer) that's isolated from the ARM async pipeline.
	// When QueueProvider is configured (production via NewOptions), create a new provider with
	// our queue name. Honor injected queue providers for tests, then fall back to in-memory
	// for minimal configurations that don't configure a provider.
	if options.QueueProvider != nil && options.QueueProvider.HasInjectedClient() {
		qClient, err = options.QueueProvider.GetClient(ctx)
		if err != nil {
			return err
		}
	} else if options.Config.Queue.Provider != "" {
		qOpts := options.Config.Queue
		qOpts.Name = QueueName
		qp := queueprovider.New(qOpts)
		qClient, err = qp.GetClient(ctx)
		if err != nil {
			return err
		}
	} else {
		// Fallback for tests and minimal configurations
		qClient = qinmem.NewNamedQueue(QueueName)
	}

	// Database client using the shared provider when configured; fallback to in-memory for tests/local.
	if options.DatabaseProvider != nil {
		dbClient, err = options.DatabaseProvider.GetClient(ctx)
		if err != nil {
			return err
		}
	} else {
		dbClient = dbinmemory.NewClient()
	}

	statusStore := NewStatusStore(dbClient, StatusStorageID)
	handler := &HTTPHandler{
		Queue:       qClient,
		StatusStore: statusStore,
		RootPath:    terraformPath(options),
	}

	basePath := strings.TrimSuffix(options.Config.Server.PathBase, "/") + "/installer/terraform"
	r.Route(basePath, func(route chi.Router) {
		route.Post("/install", handler.Install)
		route.Post("/uninstall", handler.Uninstall)
		route.Get("/status", handler.Status)
	})

	return nil
}

// HTTPHandler handles installer HTTP endpoints.
type HTTPHandler struct {
	Queue       queue.Client
	StatusStore StatusStore
	// RootPath is the root directory for Terraform installations.
	// Used to build binary paths in status responses.
	RootPath string
}

func (h *HTTPHandler) Install(w http.ResponseWriter, r *http.Request) {
	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateInstallRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	msg := JobMessage{
		Operation:  OperationInstall,
		Version:    req.Version,
		SourceURL:  req.SourceURL,
		Checksum:   req.Checksum,
		CABundle:   req.CABundle,
		AuthHeader: req.AuthHeader,
		ClientCert: req.ClientCert,
		ClientKey:  req.ClientKey,
		ProxyURL:   req.ProxyURL,
	}
	if err := h.Queue.Enqueue(r.Context(), queue.NewMessage(msg)); err != nil {
		http.Error(w, "failed to enqueue install", http.StatusInternalServerError)
		return
	}

	// Increment pending count in status (best-effort)
	h.incrementQueuePending(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "install enqueued",
		"version": req.Version,
	})
}

func (h *HTTPHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	var req UninstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// If no version specified, default to current version
	if strings.TrimSpace(req.Version) == "" {
		status, err := h.StatusStore.Get(r.Context())
		if err != nil {
			http.Error(w, "failed to get status", http.StatusInternalServerError)
			return
		}
		if status.Current == "" {
			http.Error(w, "no current version installed", http.StatusBadRequest)
			return
		}
		req.Version = status.Current
	}

	msg := JobMessage{
		Operation: OperationUninstall,
		Version:   req.Version,
		Purge:     req.Purge,
	}
	if err := h.Queue.Enqueue(r.Context(), queue.NewMessage(msg)); err != nil {
		http.Error(w, "failed to enqueue uninstall", http.StatusInternalServerError)
		return
	}

	// Increment pending count in status (best-effort)
	h.incrementQueuePending(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "uninstall enqueued",
		"version": req.Version,
	})
}

func (h *HTTPHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.StatusStore.Get(r.Context())
	if err != nil {
		http.Error(w, "failed to load status", http.StatusInternalServerError)
		return
	}

	rootPath := h.RootPath
	if rootPath == "" {
		rootPath = "/terraform"
	}
	response := status.ToResponse(rootPath)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func validateInstallRequest(req InstallRequest) error {
	version := strings.TrimSpace(req.Version)
	sourceURL := strings.TrimSpace(req.SourceURL)

	if version == "" && sourceURL == "" {
		return fmt.Errorf("version or sourceUrl is required")
	}

	if version != "" && !IsValidVersion(version) {
		return fmt.Errorf("invalid version format")
	}

	if sourceURL != "" {
		parsed, err := url.Parse(sourceURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid sourceUrl")
		}
	}

	if strings.TrimSpace(req.Checksum) != "" && !IsValidChecksum(req.Checksum) {
		return fmt.Errorf("invalid checksum format")
	}

	// Validate mTLS: both client cert and key must be provided together
	clientCert := strings.TrimSpace(req.ClientCert)
	clientKey := strings.TrimSpace(req.ClientKey)
	if (clientCert != "" && clientKey == "") || (clientCert == "" && clientKey != "") {
		return fmt.Errorf("both clientCert and clientKey must be provided for mTLS")
	}

	// Validate that download options require sourceUrl (they don't make sense for version-only installs)
	if sourceURL == "" {
		if strings.TrimSpace(req.CABundle) != "" {
			return fmt.Errorf("caBundle requires sourceUrl to be set")
		}
		if strings.TrimSpace(req.AuthHeader) != "" {
			return fmt.Errorf("authHeader requires sourceUrl to be set")
		}
		if clientCert != "" {
			return fmt.Errorf("clientCert requires sourceUrl to be set")
		}
		if strings.TrimSpace(req.ProxyURL) != "" {
			return fmt.Errorf("proxyUrl requires sourceUrl to be set")
		}
	}

	// Validate proxy URL format if provided
	if proxyURL := strings.TrimSpace(req.ProxyURL); proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Host == "" {
			return fmt.Errorf("invalid proxyUrl")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("proxyUrl must use http or https scheme")
		}
	}

	return nil
}

// terraformPath returns the configured terraform installation path from UCP options,
// defaulting to "/terraform" if not configured.
func terraformPath(options *ucp.Options) string {
	if options.Config.Terraform.Path != "" {
		return options.Config.Terraform.Path
	}
	return "/terraform"
}

// terraformPathFromHostOptions returns the configured terraform installation path from HostOptions,
// defaulting to "/terraform" if not configured.
func terraformPathFromHostOptions(options *hostoptions.HostOptions) string {
	if options.Config.Terraform.Path != "" {
		return options.Config.Terraform.Path
	}
	return "/terraform"
}

// incrementQueuePending increments the pending job count in status.
// Note: This is a best-effort metric. The count may be inaccurate if status
// updates fail or if messages are added/removed through non-standard paths.
// For exact counts, query the queue directly.
func (h *HTTPHandler) incrementQueuePending(ctx context.Context) {
	updateQueueInfo(ctx, h.StatusStore, func(q *QueueInfo) {
		q.Pending++
	})
}
