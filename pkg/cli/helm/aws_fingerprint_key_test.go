//go:build !windows

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

package helm

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/go-logr/logr"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TestAWSFingerprintKeySecretUpgrade installs the shipping AWS fingerprint key Secret, then upgrades and rolls back
// with standalone Helm and with rad. Every step must keep the Secret and its key.
func TestAWSFingerprintKeySecretUpgrade(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	helmBinary, err := exec.LookPath("helm")
	require.NoError(t, err)

	kubeconfig, client := startTestAPIServer(t)
	chartPath, chart := shippingTemplatesChart(t, "ucp/aws-fingerprint-key.yaml")

	for _, upgrader := range []string{"helm", "rad"} {
		t.Run(upgrader, func(t *testing.T) {
			namespace := "aws-fingerprint-key-" + upgrader
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig = &kubeconfig
			flags.Namespace = &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			radiusClient := NewHelmClient()
			_, err := radiusClient.RunHelmInstall(configuration, chart, nil, "radius", namespace, false, time.Minute)
			require.NoError(t, err)

			secrets := client.CoreV1().Secrets(namespace)
			installed, err := secrets.Get(t.Context(), ucp_aws.FingerprintKeySecretName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Len(t, installed.Data, 1)
			require.Len(t, installed.Data["key"], ucp_aws.MinFingerprintKeyLength)

			helmCommand := func(args ...string) {
				t.Helper()
				args = append(args, "--kubeconfig", kubeconfig, "--kube-context", "test", "--namespace", namespace, "--timeout", "1m")
				command := exec.CommandContext(t.Context(), helmBinary, args...)
				command.Env = append(os.Environ(), "HELM_DRIVER=secret")
				output, err := command.CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			requireKeptKey := func(step string) {
				t.Helper()
				current, err := secrets.Get(t.Context(), installed.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, installed.UID, current.UID, "%s must not replace the Secret.", step)
				require.Equal(t, installed.Data, current.Data, "%s must keep the key.", step)
			}

			if upgrader == "helm" {
				helmCommand("upgrade", "radius", chartPath, "--reuse-values")
				requireKeptKey("Upgrade")
				helmCommand("rollback", "radius", "1")
				requireKeptKey("Rollback")
				return
			}

			_, err = radiusClient.RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)
			requireKeptKey("Upgrade")
			require.NoError(t, radiusClient.RunHelmRollback(configuration, "radius", 1, false))
			requireKeptKey("Rollback")
		})
	}
}
