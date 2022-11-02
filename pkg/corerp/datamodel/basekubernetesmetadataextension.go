package datamodel

// BaseKubernetesMetadataExtension - Specifies base struct for user defined labels and annotations
type BaseKubernetesMetadataExtension struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}
