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

package ucp

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/sdk"
	ucpconfig "github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/validator"
	"github.com/radius-project/radius/swagger"
	kube_rest "k8s.io/client-go/rest"
)

// Options holds the configuration options and shared services for the UCP server.
//
// For testability, all fields on this struct MUST be constructed from the NewOptions function without any
// additional initialization required.
type Options struct {
	// Config is the configuration for the server.
	Config *Config

	// DatabaseProvider provides access to the database used for resource data.
	DatabaseProvider *databaseprovider.DatabaseProvider

	// Modules is the list of modules to initialize. This will default to nil (implying the default set), and
	// can be overridden by tests.
	Modules []modules.Initializer

	// QueueProvider provides access to the message queue client.
	QueueProvider *queueprovider.QueueProvider

	// SecretProvider provides access to secret store used for secret data.
	SecretProvider *secretprovider.SecretProvider

	// SpecLoader is the loader for the OpenAPI spec.
	SpecLoader *validator.Loader

	// StatusManager implements operations on async operation statuses.
	StatusManager statusmanager.StatusManager

	// UCP is the connection to UCP
	UCP sdk.Connection
}

// NewOptions creates a new Options instance from the given configuration.
func NewOptions(ctx context.Context, config *Config) (*Options, error) {
	var err error
	options := Options{
		Config: config,

		Modules: nil, // Default to nil, which implies the default set of modules.
	}

	options.DatabaseProvider = databaseprovider.FromOptions(config.Database)
	options.QueueProvider = queueprovider.New(config.Queue)
	options.SecretProvider = secretprovider.NewSecretProvider(config.Secrets)

	databaseClient, err := options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	queueClient, err := options.QueueProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	options.StatusManager = statusmanager.New(databaseClient, queueClient, config.Environment.RoleLocation)

	options.SpecLoader, err = validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, []string{config.Server.PathBase}, "")
	if err != nil {
		return nil, err
	}

	var cfg *kube_rest.Config
	if config.UCP.Kind == ucpconfig.UCPConnectionKindKubernetes {
		cfg, err = kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
			// TODO: Allow to use custom context via configuration. - https://github.com/radius-project/radius/issues/5433
			ContextName: "",
			QPS:         kubeutil.DefaultServerQPS,
			Burst:       kubeutil.DefaultServerBurst,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
		}
	}

	options.UCP, err = ucpconfig.NewConnectionFromUCPConfig(&config.UCP, cfg)
	if err != nil {
		return nil, err
	}

	return &options, nil
}
