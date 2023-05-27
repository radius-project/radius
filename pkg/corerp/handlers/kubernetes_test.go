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

package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeploymentWatcher(t *testing.T) {
	ctx := context.Background()
	deploymentName := "test-deployment"
	deploymentNamespace := "test-namespace"

	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: deploymentNamespace,
		},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Reason:  "NewReplicaSetAvailable",
					Message: "Deployment has minimum availability",
				},
			},
		},
	}

	// Create another deployment that should not be watched
	deploymentUnrelated := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated-deployment",
			Namespace: deploymentNamespace,
		},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Reason:  "NewReplicaSetAvailable",
					Message: "Deployment has minimum availability",
				},
			},
		},
	}

	deploymentClient := fake.NewSimpleClientset(deployment)
	err := deploymentClient.Tracker().Add(deploymentUnrelated)
	require.NoError(t, err, "Failed to add unrelated deployment to tracker")

	readinessCh := make(chan bool, 2)
	watchErrorCh := make(chan error)
	eventHandlerInvokedCh := make(chan struct{}, 2)
	handler := kubernetesHandler{
		clientSet: deploymentClient,
	}

	// Create a fake informer factory
	informerFactory := informers.NewSharedInformerFactory(deploymentClient, 0)

	go func() {
		// Watch the first deployment
		handler.WatchUntilReady(ctx, informerFactory, deployment, readinessCh, watchErrorCh, eventHandlerInvokedCh)
	}()

	ready := false
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Test timed out")
		case <-readinessCh:
			t.Logf("Deployment %s in namespace %s is ready", deploymentName, deploymentNamespace)
			ready = true
			break
		case err := <-watchErrorCh:
			t.Fatalf("Error occured while watching the deployment: %s", err.Error())
		}

		if ready {
			break
		}
	}

	// Make sure deploymentUnrelated was not watched. We expect no event handlers to be invoked
	// for deploymentUnrelated and therefore a single message on the eventHandlerInvokedCh
	require.Equal(t, 1, len(eventHandlerInvokedCh))
}
