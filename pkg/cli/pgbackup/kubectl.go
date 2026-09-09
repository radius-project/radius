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

package pgbackup

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/radius-project/radius/pkg/process"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func kubectlCommand(ctx context.Context, kubeContext, namespace string, args ...string) (*exec.Cmd, error) {
	if process.IsWindowless() {
		if err := validateExecAuth(kubeContext); err != nil {
			return nil, err
		}
	}

	args = append([]string{"--context", kubeContext, "-n", namespace}, args...)
	return process.CommandContext(ctx, "kubectl", args...), nil
}

func validateExecAuth(kubeContext string) error {
	// Use kubectl's file-list merging and API defaults, but never migrate or write
	// kubeconfig files during this read-only check.
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.MigrationRules = nil
	config, err := rules.Load()
	if err != nil {
		return fmt.Errorf("failed to load kubectl configuration: %w", err)
	}

	overrides := &clientcmd.ConfigOverrides{
		CurrentContext:  kubeContext,
		ClusterDefaults: clientcmd.ClusterDefaults,
	}
	// MergedRawConfig validates only the selected configuration, without creating
	// a transport, authenticating, or executing a credential plugin.
	selected, err := clientcmd.NewNonInteractiveClientConfig(*config, "", overrides, rules).MergedRawConfig()
	if err != nil {
		return fmt.Errorf("invalid kubectl configuration: %w", err)
	}

	selectedContext := selected.Contexts[selected.CurrentContext]
	auth := selected.AuthInfos[selectedContext.AuthInfo]
	if auth.Exec != nil && auth.Exec.InteractiveMode == api.AlwaysExecInteractiveMode {
		return fmt.Errorf("kubectl context %q requires interactive exec authentication (interactiveMode: Always), but Radius is running without a Windows console; configure non-interactive Kubernetes credentials or run Radius from an attached console", selected.CurrentContext)
	}
	return nil
}
