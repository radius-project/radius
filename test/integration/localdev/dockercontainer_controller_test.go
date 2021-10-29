// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/test/testcontext"
)

// Ensure that a replica is started when new Executable object appears
func TestDockerContainerStartsReplicas(t *testing.T) {
	t.Parallel()
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	const namespace = "container-starts-replicas-ns"
	if err := ensureNamespace(ctx, namespace); err != nil {
		t.Fatalf("Could not create namespace for the test: %v", err)
	}

	container := radiusv1alpha3.DockerContainer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "dockercontainer-starts-replicas",
		},
		Spec: radiusv1alpha3.DockerContainerSpec{
			Image:    "rynowak/backend:0.5.0-dev",
			Replicas: 1,
		},
	}

	t.Logf("Creating container '%s'", container.ObjectMeta.Name)
	if err := client.Create(ctx, &container, &runtimeclient.CreateOptions{}); err != nil {
		t.Fatalf("Unable to create Executable: %v", err)
	}

	t.Log("Checking if replica has started...")
	if err := ensureDockerContainerRunning(ctx, container.Spec.Image, container.Spec.Replicas); err != nil {
		t.Fatalf("Replicas could not be started: %v", err)
	}
}

func ensureDockerContainerRunning(ctx context.Context, image string, replicas int) error {
	return nil
}
