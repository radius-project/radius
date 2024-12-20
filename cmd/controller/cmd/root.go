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
	"fmt"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	"github.com/radius-project/radius/pkg/controller"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/spf13/cobra"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

var rootCmd = &cobra.Command{
	Use:   "controller",
	Short: "Radius Kubernetes controller",
	Long:  `Server process for Radius Kubernetes interoperability (controller).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFilePath := cmd.Flag("config-file").Value.String()
		tlsCertDir := cmd.Flag("cert-dir").Value.String()

		options, err := hostoptions.NewHostOptionsFromEnvironment(configFilePath)
		if err != nil {
			return err
		}

		logger, flush, err := ucplog.NewLogger("controller", &options.Config.Logging)
		if err != nil {
			return err
		}
		defer flush()

		ctrl.SetLogger(logger)
		runtimelog.SetLogger(logger)

		ctx := logr.NewContext(context.Background(), logger)

		logger.Info("Loaded options", "configfile", configFilePath)

		services := []hosting.Service{
			&controller.Service{Options: options, TLSCertDir: tlsCertDir},
		}

		if options.Config.TracerProvider.Enabled {
			services = append(services, &traceservice.Service{Options: &options.Config.TracerProvider})
		}

		host := &hosting.Host{Services: services}
		return hosting.RunWithInterrupts(ctx, host)
	},
}

func Execute() {
	// Let users override the configuration via `--config-file`.
	rootCmd.Flags().String("config-file", fmt.Sprintf("controller-%s.yaml", hostoptions.Environment()), "The service configuration file.")
	rootCmd.Flags().String("cert-dir", "/var/tls/cert", "The directory containing the TLS certificates.")

	cobra.CheckErr(rootCmd.ExecuteContext(context.Background()))
}
