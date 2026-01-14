/*
Copyright 2024 The Radius Authors.

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

	"github.com/radius-project/radius/pkg/components/database"
	dbinmemory "github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	qinmem "github.com/radius-project/radius/pkg/components/queue/inmemory"
	"github.com/radius-project/radius/pkg/ucp"
)

// RegisterRoutes registers installer endpoints on the UCP router.
func RegisterRoutes(ctx context.Context, r chi.Router, options *ucp.Options) error {
	var (
		qClient  queue.Client
		dbClient database.Client
		err      error
	)

	// Queue client using the shared provider when configured; fallback to in-memory for tests/local.
	if options.QueueProvider != nil {
		qClient, err = options.QueueProvider.GetClient(ctx)
		if err != nil {
			return err
		}
	} else {
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
		Operation: OperationInstall,
		Version:   req.Version,
		SourceURL: req.SourceURL,
		Checksum:  req.Checksum,
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

	return nil
}

// terraformPath returns the configured terraform installation path from options,
// defaulting to "/terraform" if not configured.
func terraformPath(options *ucp.Options) string {
	if options.Config.Terraform.Path != "" {
		return options.Config.Terraform.Path
	}
	return "/terraform"
}

// incrementQueuePending increments the pending count in status (best-effort, logs on failure).
func (h *HTTPHandler) incrementQueuePending(ctx context.Context) {
	updateQueueInfo(ctx, h.StatusStore, func(q *QueueInfo) {
		q.Pending++
	})
}
