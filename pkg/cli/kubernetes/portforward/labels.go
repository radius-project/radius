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
	"github.com/radius-project/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func CreateLabelSelectorForApplication(applicationName string) (labels.Selector, error) {
	applicationLabel, err := labels.NewRequirement(kubernetes.LabelRadiusApplication, selection.Equals, []string{applicationName})
	if err != nil {
		return nil, err
	}

	return labels.NewSelector().Add(*applicationLabel), nil
}

func CreateLabelSelectorForDashboard() (labels.Selector, error) {
	dashboardNameLabel, err := labels.NewRequirement(kubernetes.LabelName, selection.Equals, []string{"dashboard"})
	if err != nil {
		return nil, err
	}

	dashboardPartOfLabel, err := labels.NewRequirement(kubernetes.LabelPartOf, selection.Equals, []string{"radius"})
	if err != nil {
		return nil, err
	}

	return labels.NewSelector().Add(*dashboardNameLabel).Add(*dashboardPartOfLabel), nil
}

func CreateLabelsForDashboard() labels.Labels {
	return labels.Set{
		kubernetes.LabelName:   "dashboard",
		kubernetes.LabelPartOf: "radius",
	}
}
