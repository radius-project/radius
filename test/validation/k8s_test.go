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
package validation

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_compareActualAndExpectedResources(t *testing.T) {
	type args struct {
		expectedResources []K8sObject
		actualResources   []unstructured.Unstructured
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Dapr Pub Sub Broker Manual Deployment - Success",
			args: args{
				expectedResources: []K8sObject{
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "pods",
						},
						Kind:         "Pod",
						ResourceName: "dpsb-manual-app-ctnr",
						Labels: map[string]string{
							"radius.dev/application":   "dpsb-manual-app",
							"radius.dev/resource":      "dpsb-manual-app-ctnr",
							"radius.dev/resource-type": "applications.core-containers",
						},
						Source: SourceRadius,
					},
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "pods",
						},
						Kind:         "Pod",
						ResourceName: "dpsb-manual-redis",
						Source:       SourceRadius,
					},
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "services",
						},
						Kind:         "Service",
						ResourceName: "dpsb-manual-redis",
						Source:       SourceRadius,
					},
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "dapr.io",
							Version:  "v1alpha1",
							Resource: "components",
						},
						Kind:         "Component",
						ResourceName: "dpsb-manual",
						Source:       SourceRadius,
					},
				},
				actualResources: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]any{
								"name":      "dpsb-manual-app-ctnr-f57c99cbd-gk2sw",
								"namespace": "default",
								"labels": map[string]any{
									"radius.dev/application":   "dpsb-manual-app",
									"radius.dev/resource":      "dpsb-manual-app-ctnr",
									"radius.dev/resource-type": "applications.core-containers",
								},
							},
						},
					},
					{
						Object: map[string]any{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]any{
								"name":      "dpsb-manual-redis",
								"namespace": "default",
							},
						},
					},
					{
						Object: map[string]any{
							"kind":       "Service",
							"apiVersion": "v1",
							"metadata": map[string]any{
								"name":      "dpsb-manual-redis",
								"namespace": "default",
							},
						},
					},
					{
						Object: map[string]any{
							"kind":       "Component",
							"apiVersion": "dapr.io/v1alpha1",
							"metadata": map[string]any{
								"name":      "dpsb-manual",
								"namespace": "default",
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "Dapr Pub Sub Broker Manual Deployment - Missing Resource",
			args: args{
				expectedResources: []K8sObject{
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "pods",
						},
						Kind:         "Pod",
						ResourceName: "dpsb-manual-app-ctnr",
						Labels: map[string]string{
							"radius.dev/application":   "dpsb-manual-app",
							"radius.dev/resource":      "dpsb-manual-app-ctnr",
							"radius.dev/resource-type": "applications.core-containers",
						},
						Source: SourceRadius,
					},
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "pods",
						},
						Kind:         "Pod",
						ResourceName: "dpsb-manual-redis",
						Source:       SourceRadius,
					},
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "services",
						},
						Kind:         "Service",
						ResourceName: "dpsb-manual-redis",
						Source:       SourceRadius,
					},
					{
						GroupVersionResource: schema.GroupVersionResource{
							Group:    "dapr.io",
							Version:  "v1alpha1",
							Resource: "components",
						},
						Kind:         "Component",
						ResourceName: "dpsb-manual",
						Source:       SourceRadius,
					},
				},
				actualResources: []unstructured.Unstructured{
					{
						Object: map[string]any{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]any{
								"name":      "dpsb-manual-app-ctnr-f57c99cbd-gk2sw",
								"namespace": "default",
								"labels": map[string]any{
									"radius.dev/application":   "dpsb-manual-app",
									"radius.dev/resource":      "dpsb-manual-app-ctnr",
									"radius.dev/resource-type": "applications.core-containers",
								},
							},
						},
					},
					{
						Object: map[string]any{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]any{
								"name":      "dpsb-manual-redis",
								"namespace": "default",
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareActualAndExpectedResources(tt.args.expectedResources, tt.args.actualResources); got != tt.want {
				t.Errorf("compareActualAndExpectedResources() = %v, want %v", got, tt.want)
			}
		})
	}
}
