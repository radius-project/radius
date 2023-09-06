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
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func mergeLabelSelector(base *metav1.LabelSelector, cur *metav1.LabelSelector) *metav1.LabelSelector {
	if base == nil {
		base = &metav1.LabelSelector{}
	}

	return &metav1.LabelSelector{
		MatchLabels:      labels.Merge(base.MatchLabels, cur.MatchLabels),
		MatchExpressions: append(base.MatchExpressions, cur.MatchExpressions...),
	}
}

func mergeObjectMeta(base metav1.ObjectMeta, cur metav1.ObjectMeta) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        cur.Name,
		Namespace:   cur.Namespace,
		Labels:      labels.Merge(base.Labels, cur.Labels),
		Annotations: labels.Merge(base.Annotations, cur.Annotations),
	}
}

func getObjectMeta(base metav1.ObjectMeta, appName, resourceName, resourceType string, options renderers.RenderOptions) metav1.ObjectMeta {
	cur := metav1.ObjectMeta{
		Name:        kubernetes.NormalizeResourceName(resourceName),
		Namespace:   options.Environment.Namespace,
		Labels:      renderers.GetLabels(options, appName, resourceName, resourceType),
		Annotations: renderers.GetAnnotations(options),
	}

	return mergeObjectMeta(base, cur)
}
