/*
Copyright 2024 The Radius Authors.
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

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

func TestGitRepositoryRevisionChangePredicate_Create(t *testing.T) {
	predicate := GitRepositoryRevisionChangePredicate{}

	tests := []struct {
		name     string
		event    event.CreateEvent
		expected bool
	}{
		{
			name: "Source is not a sourcev1.Source",
			event: event.CreateEvent{
				Object: &corev1.Pod{},
			},
			expected: false,
		},
		{
			name: "Source has no artifact",
			event: event.CreateEvent{
				Object: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Source has an artifact",
			event: event.CreateEvent{
				Object: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
					Status: sourcev1.GitRepositoryStatus{
						Artifact: &sourcev1.Artifact{
							Revision: "test-revision",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := predicate.Create(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitRepositoryRevisionChangePredicate_Update(t *testing.T) {
	predicate := GitRepositoryRevisionChangePredicate{}

	tests := []struct {
		name     string
		event    event.UpdateEvent
		expected bool
	}{
		{
			name: "Source ObjectOld is nil",
			event: event.UpdateEvent{
				ObjectNew: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Source ObjectNew is nil",
			event: event.UpdateEvent{
				ObjectOld: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Source ObjectOld is not a sourcev1.Source",
			event: event.UpdateEvent{
				ObjectOld: &corev1.Pod{},
				ObjectNew: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Source ObjectNew is not a sourcev1.Source",
			event: event.UpdateEvent{
				ObjectOld: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
				ObjectNew: &corev1.Pod{},
			},
			expected: false,
		},
		{
			name: "Sources ObjectOld and ObjectNew have no artifact",
			event: event.UpdateEvent{
				ObjectOld: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
				ObjectNew: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Source ObjectNew and ObjectOld are the same",
			event: event.UpdateEvent{
				ObjectOld: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
					Status: sourcev1.GitRepositoryStatus{
						Artifact: &sourcev1.Artifact{
							Revision: "test-revision",
						},
					},
				},
				ObjectNew: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
					Status: sourcev1.GitRepositoryStatus{
						Artifact: &sourcev1.Artifact{
							Revision: "test-revision",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Source ObjectNew and ObjectOld have different revisions",
			event: event.UpdateEvent{
				ObjectOld: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
					Status: sourcev1.GitRepositoryStatus{
						Artifact: &sourcev1.Artifact{
							Revision: "test-revision",
						},
					},
				},
				ObjectNew: &sourcev1.GitRepository{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-repo",
					},
					Status: sourcev1.GitRepositoryStatus{
						Artifact: &sourcev1.Artifact{
							Revision: "test-revision-different",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := predicate.Update(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}
