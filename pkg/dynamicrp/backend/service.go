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

package backend

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/dynamicrp"
)

// Service runs the backend for the dynamic-rp.
type Service struct {
	worker.Service
	options *dynamicrp.Options
}

// NewService creates a new service to run the dynamic-rp backend.
func NewService(options *dynamicrp.Options) *Service {
	return &Service{
		options: options,
		Service: worker.Service{
			ProviderName: "dynamic-rp",
			Options: hostoptions.HostOptions{
				Config: &hostoptions.ProviderConfig{
					Env:             options.Config.Environment,
					StorageProvider: options.Config.Storage,
					SecretProvider:  options.Config.Secrets,
					QueueProvider:   options.Config.Queue,
				},
			},
		},
	}
}

// Name returns the name of the service used for logging.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", w.Service.ProviderName)
}

// Run runs the service.
func (w *Service) Run(ctx context.Context) error {
	err := w.Init(ctx)
	if err != nil {
		return err
	}

	workerOptions := worker.Options{}
	if w.options.Config.Worker.MaxOperationConcurrency != nil {
		workerOptions.MaxOperationConcurrency = *w.options.Config.Worker.MaxOperationConcurrency
	}
	if w.options.Config.Worker.MaxOperationRetryCount != nil {
		workerOptions.MaxOperationRetryCount = *w.options.Config.Worker.MaxOperationRetryCount
	}

	return w.Start(ctx, workerOptions)
}
