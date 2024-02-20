package portforward

import (
	"github.com/radius-project/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func GetDashboardSelector() (labels.Selector, error) {
	dashboardNameLabel, err := labels.NewRequirement(kubernetes.LabelName, selection.Equals, []string{"dashboard"})
	if err != nil {
		return nil, err
	}

	dashboardPartOfLabel, err := labels.NewRequirement(kubernetes.LabelPartOf, selection.Equals, []string{"radius"})
	if err != nil {
		return nil, err
	}

	dashboardSelector := labels.NewSelector().Add(*dashboardNameLabel).Add(*dashboardPartOfLabel)
	return dashboardSelector, nil
}
