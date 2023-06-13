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
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitUntilDeploymentIsReady_NewResource(t *testing.T) {
	ctx := context.TODO()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
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

	handler := kubernetesHandler{
		clientSet:           deploymentClient,
		deploymentTimeOut:   time.Duration(5) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.waitUntilDeploymentIsReady(ctx, deployment)
	require.NoError(t, err, "Failed to wait for deployment to be ready")
}

func TestWaitUntilDeploymentIsReady_Timeout(t *testing.T) {
	ctx := context.TODO()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "NewReplicaSetAvailable",
					Message: "Deployment has minimum availability",
				},
			},
		},
	}

	deploymentClient := fake.NewSimpleClientset(deployment)

	handler := kubernetesHandler{
		clientSet:           deploymentClient,
		deploymentTimeOut:   time.Duration(1) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.waitUntilDeploymentIsReady(ctx, deployment)
	require.Error(t, err)
	require.Equal(t, "deployment timed out, name: test-deployment, namespace test-namespace, status: Deployment has minimum availability, reason: NewReplicaSetAvailable", err.Error())
}

func TestWaitUntilDeploymentIsReady_DifferentResourceName(t *testing.T) {
	ctx := context.TODO()
	// Create first deployment that will be watched
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
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

	handler := kubernetesHandler{
		clientSet:           deploymentClient,
		deploymentTimeOut:   time.Duration(1) * time.Second,
		cacheResyncInterval: time.Duration(10) * time.Second,
	}

	err := handler.waitUntilDeploymentIsReady(ctx, &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-matched-deployment",
			Namespace: "test-namespace",
		},
	})

	// It must be timed out because the name of the deployment does not match.
	require.Error(t, err)
	require.Equal(t, "deployment timed out, name: not-matched-deployment, namespace test-namespace, error occured while fetching latest status: deployments.apps \"not-matched-deployment\" not found", err.Error())
}
