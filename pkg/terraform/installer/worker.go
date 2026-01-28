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
	"errors"
	"time"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// WorkerService runs the installer queue consumer in the UCP host.
// It uses a dedicated queue so Terraform binary install/uninstall jobs stay isolated from the ARM async pipeline,
// which expects ARM operation payloads and semantics.
type WorkerService struct {
	options *ucp.Options
}

// NewWorkerService creates a new WorkerService.
func NewWorkerService(options *ucp.Options) *WorkerService {
	return &WorkerService{options: options}
}

// Name returns the service name.
func (s *WorkerService) Name() string {
	return "terraform-installer-worker"
}

// Run starts consuming installer queue messages.
func (s *WorkerService) Run(ctx context.Context) error {
	log := ucplog.FromContextOrDiscard(ctx)

	dbProvider := databaseprovider.FromOptions(s.options.Config.Database)
	dbClient, err := dbProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	qOpts := s.options.Config.Queue
	qOpts.Name = QueueName
	qp := queueprovider.New(qOpts)
	queueClient, err := qp.GetClient(ctx)
	if err != nil {
		return err
	}

	statusStore := NewStatusStore(dbClient, StatusStorageID)
	handler := &Handler{
		StatusStore: statusStore,
		RootPath:    s.terraformPath(),
		BaseURL:     s.options.Config.Terraform.SourceBaseURL,
	}

	msgCh, err := queue.StartDequeuer(ctx, queueClient, queue.WithDequeueInterval(time.Second*2))
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgCh:
			if !ok {
				return nil
			}

			if err := handler.Handle(ctx, msg); err != nil {
				if errors.Is(err, ErrInstallerBusy) {
					log.Info("installer busy; recording failure for request", "messageID", msg.ID)

					// Extract version from message for failure recording
					var job JobMessage
					if decodeErr := json.Unmarshal(msg.Data, &job); decodeErr != nil {
						log.Error(decodeErr, "failed to decode job message for failure recording")
					} else if job.Version == "" {
						// Skip recording failure for empty version to avoid polluting status map
						log.Info("skipping failure recording for job with empty version")
					} else {
						status, getErr := handler.getOrInitStatus(ctx)
						if getErr != nil {
							log.Error(getErr, "failed to load status while handling busy installer")
						} else {
							_ = handler.recordFailure(ctx, status, job.Version, err)
						}
					}
				} else {
					log.Error(err, "failed to handle installer message")
				}
			}

			// FinishMessage removes the message from the queue. If this fails, the message
			// will be redelivered after the queue's visibility timeout expires. The queue's
			// built-in retry and dead-letter mechanisms will handle repeated failures.
			if err := queueClient.FinishMessage(ctx, msg); err != nil {
				log.Error(err, "failed to finish installer message", "messageID", msg.ID)
			}
		}
	}
}

func (s *WorkerService) terraformPath() string {
	if s.options.Config.Terraform.Path != "" {
		return s.options.Config.Terraform.Path
	}
	return "/terraform"
}

// HostOptionsWorkerService runs the installer queue consumer using HostOptions.
// This is used by applications-rp instead of UCP.
type HostOptionsWorkerService struct {
	options hostoptions.HostOptions
}

// NewHostOptionsWorkerService creates a new HostOptionsWorkerService.
func NewHostOptionsWorkerService(options hostoptions.HostOptions) *HostOptionsWorkerService {
	return &HostOptionsWorkerService{options: options}
}

// Name returns the service name.
func (s *HostOptionsWorkerService) Name() string {
	return "terraform-installer-worker"
}

// Run starts consuming installer queue messages.
func (s *HostOptionsWorkerService) Run(ctx context.Context) error {
	log := ucplog.FromContextOrDiscard(ctx)

	dbProvider := databaseprovider.FromOptions(s.options.Config.DatabaseProvider)
	dbClient, err := dbProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	qOpts := s.options.Config.QueueProvider
	qOpts.Name = QueueName
	qp := queueprovider.New(qOpts)
	queueClient, err := qp.GetClient(ctx)
	if err != nil {
		return err
	}

	statusStore := NewStatusStore(dbClient, StatusStorageID)
	handler := &Handler{
		StatusStore: statusStore,
		RootPath:    s.hostOptionsPath(),
		BaseURL:     s.options.Config.Terraform.SourceBaseURL,
	}

	msgCh, err := queue.StartDequeuer(ctx, queueClient, queue.WithDequeueInterval(time.Second*2))
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgCh:
			if !ok {
				return nil
			}

			if err := handler.Handle(ctx, msg); err != nil {
				if errors.Is(err, ErrInstallerBusy) {
					log.Info("installer busy; recording failure for request", "messageID", msg.ID)

					// Extract version from message for failure recording
					var job JobMessage
					if decodeErr := json.Unmarshal(msg.Data, &job); decodeErr != nil {
						log.Error(decodeErr, "failed to decode job message for failure recording")
					} else if job.Version == "" {
						// Skip recording failure for empty version to avoid polluting status map
						log.Info("skipping failure recording for job with empty version")
					} else {
						status, getErr := handler.getOrInitStatus(ctx)
						if getErr != nil {
							log.Error(getErr, "failed to load status while handling busy installer")
						} else {
							_ = handler.recordFailure(ctx, status, job.Version, err)
						}
					}
				} else {
					log.Error(err, "failed to handle installer message")
				}
			}

			// FinishMessage removes the message from the queue. If this fails, the message
			// will be redelivered after the queue's visibility timeout expires. The queue's
			// built-in retry and dead-letter mechanisms will handle repeated failures.
			if err := queueClient.FinishMessage(ctx, msg); err != nil {
				log.Error(err, "failed to finish installer message", "messageID", msg.ID)
			}
		}
	}
}

func (s *HostOptionsWorkerService) hostOptionsPath() string {
	if s.options.Config.Terraform.Path != "" {
		return s.options.Config.Terraform.Path
	}
	return "/terraform"
}
