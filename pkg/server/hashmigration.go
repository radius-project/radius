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

package server

import (
	"context"
	"errors"
	"os"

	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/corerp/renderers/container"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

// HashMigrationComponent is the component name applications-rp records its SHA-256 migration status under.
const HashMigrationComponent = "applications-rp"

// applicationsRPApp is the app.kubernetes.io/name label of the Applications RP pods, and the name of their container.
const applicationsRPApp = "applications-rp"

// NewHashMigrationService creates the service that migrates the SHA-1-derived values applications-rp owns: the
// Terraform states of its resources, and the secret hashes of the workloads of its containers.
func NewHashMigrationService(options hostoptions.HostOptions) *hashmigration.Service {
	newClientset := func() (*kubernetes.Clientset, error) {
		if options.K8sConfig == nil {
			return nil, errors.New("the Kubernetes configuration of the control plane is not available")
		}

		return kubernetes.NewForConfig(options.K8sConfig)
	}

	controlPlane := func() (kubernetes.Interface, error) {
		client, err := newClientset()
		if err != nil {
			return nil, err
		}

		return client, nil
	}

	target := func(ctx context.Context) (kubernetes.Interface, error) {
		client, err := newClientset()
		if err != nil {
			return nil, err
		}

		// Containers are deployed to the deployment target, which can be an external cluster.
		clients, err := new(AsyncWorker).deploymentTargetClients(ctx, &kubeutil.Clients{ClientSet: client})
		if err != nil {
			return nil, err
		}

		return clients.ClientSet, nil
	}

	var reporter hashmigration.Reporter
	if client, err := controlPlane(); err == nil {
		reporter = hashmigration.NewConfigMapReporter(client, hashmigration.DefaultNamespace)
	}

	return newHashMigrationService(databaseprovider.FromOptions(options.Config.DatabaseProvider), controlPlane, target, reporter)
}

func newHashMigrationService(databaseProvider *databaseprovider.DatabaseProvider, controlPlane func() (kubernetes.Interface, error), target func(context.Context) (kubernetes.Interface, error), reporter hashmigration.Reporter) *hashmigration.Service {
	// The Applications RP pods run in the control-plane cluster. The host name of a pod is its name.
	otherReleases := func(ctx context.Context) ([]string, error) {
		client, err := controlPlane()
		if err != nil {
			return nil, err
		}

		self, err := os.Hostname()
		if err != nil {
			return nil, err
		}

		return hashmigration.OtherReleasePods(ctx, client, hashmigration.DefaultNamespace, applicationsRPApp, self)
	}

	return hashmigration.NewService(hashmigration.Options{
		Component: HashMigrationComponent,
		Steps: []hashmigration.Step{
			// Terraform stores its states in the control-plane cluster.
			backends.NewStateMigrationStep(controlPlane, databaseProvider.GetClient),
			container.NewSecretHashMigrationStep(target, otherReleases),
		},
		Reporter:     reporter,
		InitialDelay: hashmigration.DefaultInitialDelay,
	})
}
