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

package cmd

import (
	"context"
	"log"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	etcdclient "go.etcd.io/etcd/client/v3"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/server"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var rootCmd = &cobra.Command{
	Use:   "ucpd",
	Short: "UCP server",
	Long:  `Server process for the Univeral Control Plane (UCP).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		options, err := server.NewServerOptionsFromEnvironment()
		if err != nil {
			return err
		}

		logger, flush, err := ucplog.NewLogger(ucplog.LoggerName, &options.LoggingOptions)
		if err != nil {
			log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
		}
		defer flush()

		// Must set the logger before using controller-runtime.
		runtimelog.SetLogger(logger)

		if options.StorageProviderOptions.Provider == dataprovider.TypeETCD &&
			options.StorageProviderOptions.ETCD.InMemory {
			// For in-memory etcd we need to register another service to manage its lifecycle.
			//
			// The client will be initialized asynchronously.
			clientconfigSource := hosting.NewAsyncValue[etcdclient.Client]()
			options.StorageProviderOptions.ETCD.Client = clientconfigSource
			options.SecretProviderOptions.ETCD.Client = clientconfigSource
		}

		host, err := server.NewServer(&options)
		if err != nil {
			return err
		}

		ctx := logr.NewContext(cmd.Context(), logger)

		return hosting.RunWithInterrupts(ctx, host)
	},
}

func Execute() {
	cobra.CheckErr(rootCmd.ExecuteContext(context.Background()))
}
