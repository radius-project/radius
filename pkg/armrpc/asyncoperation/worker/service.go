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

package worker

import (
	"context"
	"sync"

	manager "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Service is the base worker service implementation to initialize and start worker.
// All exported fields should be initialized by the caller.
type Service struct {
	// DatabaseClient is database client.
	DatabaseClient database.Client

	// OperationStatusManager is the manager of the operation status.
	OperationStatusManager manager.StatusManager

	// Options configures options for the async worker.
	Options Options

	// QueueProvider is the queue client.
	QueueClient queue.Client

	// controllers is the registry of the async operation controllers.
	controllers *ControllerRegistry

	// controllersInit is used to ensure single initialization of controllers.
	controllersInit sync.Once
}

// Controllers returns the controller registry for the worker service.
func (s *Service) Controllers() *ControllerRegistry {
	s.controllersInit.Do(func() {
		s.controllers = NewControllerRegistry()
	})

	return s.controllers
}

// Start creates and starts a worker, and logs any errors that occur while starting the worker.
func (s *Service) Start(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Create and start worker.
	worker := New(s.Options, s.OperationStatusManager, s.QueueClient, s.Controllers())

	logger.Info("Start Worker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
		return err
	}

	logger.Info("Worker stopped...")
	return nil
}
