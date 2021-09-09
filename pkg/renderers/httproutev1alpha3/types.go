package httproutev1alpha3

const (
	Kind = "HttpRoute"
)

type HttpRoute struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Port   *int   `json:"port"`
	Url    string `json:"url"`
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
}

func (h HttpRoute) GetEffectivePort() int {
	if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}
