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

package portforward

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_findStaleReplicaSets(t *testing.T) {
	objs := []runtime.Object{

		// Owned by d1
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs1a",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d1",
					},
				},
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "1",
				},
			},
		},

		// Also owned by d1, but newer revision
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs1b",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d1",
					},
				},
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "3",
				},
			},
		},

		// Also owned by d1, but newer revision (not newest, though)
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs1c",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d1",
					},
				},
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "2",
				},
			},
		},

		// Owned by d2 - only one replicaset is here, so it can't be stale
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs2a",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d2",
					},
				},
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "3",
				},
			},
		},

		// No owner, ignored
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs3a",
				Namespace: "default",
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "3",
				},
			},
		},

		// Not part of application, ignored
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs4a",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d4",
					},
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "3",
				},
			},
		},

		// Part of other application, ignored
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "rs5a",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d5",
					},
				},
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "another-test-app",
				},
				Annotations: map[string]string{
					"deployment.kubernetes.io/revision": "3",
				},
			},
		},
	}

	expected := map[string]bool{
		"rs1a": true,
		"rs1c": true,
	}

	client := fake.NewSimpleClientset(objs...)
	actual, err := findStaleReplicaSets(context.Background(), client, "default", "test-app", "3")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
