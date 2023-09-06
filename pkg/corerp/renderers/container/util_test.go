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

package container

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeLabelSelector(t *testing.T) {
	labelMergeTests := []struct {
		name     string
		base     *metav1.LabelSelector
		cur      *metav1.LabelSelector
		expected *metav1.LabelSelector
	}{
		{
			name: "base is nil",
			base: nil,
			cur: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key2",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"value2"},
					},
				},
			},
			expected: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key2",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"value2"},
					},
				},
			},
		},
		{
			name: "base includes matchLabels",
			base: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
				},
			},
			cur: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key2": "value2",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key2",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"value2"},
					},
				},
			},
			expected: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key2",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"value2"},
					},
				},
			},
		},
	}

	for _, tc := range labelMergeTests {
		t.Run(tc.name, func(t *testing.T) {
			actual := mergeLabelSelector(tc.base, tc.cur)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestMergeObjectMeta(t *testing.T) {
	mergeObjectMetaTests := []struct {
		name     string
		base     metav1.ObjectMeta
		cur      metav1.ObjectMeta
		expected metav1.ObjectMeta
	}{
		{
			name: "base is empty",
			base: metav1.ObjectMeta{},
			cur: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
				Labels: map[string]string{
					"key1": "value1",
				},
				Annotations: map[string]string{
					"key2": "value2",
				},
			},
			expected: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
				Labels: map[string]string{
					"key1": "value1",
				},
				Annotations: map[string]string{
					"key2": "value2",
				},
			},
		},
		{
			name: "override name and namespace",
			base: metav1.ObjectMeta{
				Name:      "base",
				Namespace: "base namespace",
				Labels: map[string]string{
					"key1": "value1",
				},
				Annotations: map[string]string{
					"key1": "value1",
				},
			},
			cur: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
				Labels: map[string]string{
					"key2": "value2",
				},
				Annotations: map[string]string{
					"key2": "value2",
				},
			},
			expected: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				Annotations: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
	}

	for _, tc := range mergeObjectMetaTests {
		t.Run(tc.name, func(t *testing.T) {
			actual := mergeObjectMeta(tc.base, tc.cur)
			require.Equal(t, tc.expected, actual)
		})
	}
}
