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

// Package resource_test contains multi-cluster functional tests. These tests
// verify that an application deployed to an environment configured with an
// injected target kubeconfig (global.targetCluster.enabled) lands on an external
// (workload) cluster, while the Radius control plane runs on a separate cluster.
// Coverage spans all three deployment paths: Bicep recipes (Deployment Engine),
// Terraform recipes (cluster access resolver), and directly-rendered output
// resources (the applications-rp async worker).
//
// The tests are gated on the RADIUS_TEST_EXTERNAL_KUBECONFIG environment
// variable, which must point at the kubeconfig of the external cluster. When the
// variable is unset the tests skip, so they are inert in single-cluster runs.
package resource_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	kuberneteskeys "github.com/radius-project/radius/pkg/kubernetes"
)

// externalKubeconfigEnvVar is the environment variable the test harness sets to
// the path of the external (workload) cluster's kubeconfig. It mirrors the
// RADIUS_TARGET_KUBECONFIG contract Radius itself honors, but is consumed by the
// test (not Radius) to assert resources landed on the external cluster.
const externalKubeconfigEnvVar = "RADIUS_TEST_EXTERNAL_KUBECONFIG"

// externalClusterClients holds clients for the external (workload) cluster that
// recipes deploy to.
type externalClusterClients struct {
	clientset     *k8s.Clientset
	dynamicClient dynamic.Interface
}

// requireExternalCluster skips the test when RADIUS_TEST_EXTERNAL_KUBECONFIG is
// unset, and otherwise builds clients for the external cluster from the
// kubeconfig at that path.
func requireExternalCluster(t *testing.T) externalClusterClients {
	t.Helper()

	kubeconfigPath := os.Getenv(externalKubeconfigEnvVar)
	if kubeconfigPath == "" {
		t.Skipf("skipping multi-cluster test: %s is not set", externalKubeconfigEnvVar)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	require.NoErrorf(t, err, "failed to load external kubeconfig from %s=%q", externalKubeconfigEnvVar, kubeconfigPath)

	clientset, err := k8s.NewForConfig(config)
	require.NoError(t, err, "failed to create external cluster clientset")

	dynamicClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err, "failed to create external cluster dynamic client")

	return externalClusterClients{clientset: clientset, dynamicClient: dynamicClient}
}

// requireNoPodsForResource asserts that the control-plane cluster has no pods
// carrying the selector labels for the given application and resource. It is the
// negative half of the multi-cluster assertion: a recipe-provisioned workload
// must land on the external cluster and must NOT appear on the control plane.
func requireNoPodsForResource(ctx context.Context, t *testing.T, clientset *k8s.Clientset, namespace, application, resourceName string) {
	t.Helper()

	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: kuberneteskeys.MakeSelectorLabels(application, resourceName),
	})

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	require.NoError(t, err, "failed to list pods on control-plane cluster")
	require.Emptyf(t, pods.Items,
		"expected no pods for %s/%s on the control-plane cluster (namespace %s); the workload must land only on the external cluster",
		application, resourceName, namespace)
}
