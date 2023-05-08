/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package portforward

import (
	"context"
	"testing"
	"time"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_findStaleReplicaSets(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour * 1)

	objs := []runtime.Object{

		// Owned by d1
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs1a",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now),
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
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs1b",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(later),
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
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs1c",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(later),
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
			},
		},

		// Owned by d2 - only one replicaset is here, so it can't be stale
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs2a",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now),
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
			},
		},

		// No owner, ignored
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs3a",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now),

				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
			},
		},

		// Not part of application, ignored
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs4a",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d4",
					},
				},
			},
		},

		// Part of other application, ignored
		&appsv1.ReplicaSet{
			ObjectMeta: v1.ObjectMeta{
				Name:              "rs5a",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(now),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "d5",
					},
				},
				Labels: map[string]string{
					kubernetes.LabelRadiusApplication: "test-app",
				},
			},
		},
	}

	expected := map[string]bool{
		"rs1a": true,
		"rs1c": true,
	}

	client := fake.NewSimpleClientset(objs...)
	actual, err := findStaleReplicaSets(context.Background(), client, "default", "test-app")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
