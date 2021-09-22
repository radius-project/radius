package httproutev1alpha3

import "github.com/Azure/radius/pkg/azure/azresources"

const (
	Kind = "HttpRoute"
)

type HttpRoute struct {
	ResourceName string                 `json:"name"`
	ResourceID   azresources.ResourceID `json:"id"`
	Port         *int                   `json:"port"`
	Url          string                 `json:"url"`
	Host         string                 `json:"host"`
	Scheme       string                 `json:"scheme"`
}

func (h HttpRoute) GetEffectivePort() int {
	if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}
