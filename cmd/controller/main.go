/*
Copyright 2023.

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

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/controller"
	"github.com/radius-project/radius/pkg/trace"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/spf13/pflag"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

func main() {
	config := fmt.Sprintf("controller-%s.yaml", hostoptions.Environment())
	pflag.StringVar(&config, "config-file", config, "The service configuration file.")

	tlsCertDir := ""
	pflag.StringVar(&tlsCertDir, "cert-dir", "", "The directory containing the TLS certificates.")

	pflag.Parse()
	options, err := hostoptions.NewHostOptionsFromEnvironment(config)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}

	logger, flush, err := ucplog.NewLogger("controller", &options.Config.Logging)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}
	defer flush()

	ctrl.SetLogger(logger)
	runtimelog.SetLogger(logger)

	ctx := logr.NewContext(context.Background(), logger)

	logger.Info("Loaded options", "configfile", config)

	host := &hosting.Host{Services: []hosting.Service{
		&trace.Service{Options: options.Config.TracerProvider},
		&controller.Service{Options: options, TLSCertDir: tlsCertDir},
	}}

	err = hosting.RunWithInterrupts(ctx, host)

	// Finished shutting down. An error returned here is a failure to terminate
	// gracefully, so just crash if that happens.
	if err == nil {
		os.Exit(0) //nolint:forbidigo // this is OK inside the main function.
	} else {
		panic(err)
	}
}
