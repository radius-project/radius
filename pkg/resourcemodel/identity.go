// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcemodel

import (
	"encoding/json"
	"fmt"

	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"go.mongodb.org/mongo-driver/bson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Providers supported by Radius
// The RP will be able to support a resource only if the corresponding provider is configured with the RP
const (
	ProviderAzure = "azure"
	// This is a special case for support AAD Pod Identity which is not an ARM resource but a modification of an AKS Cluster
	ProviderAzureKubernetesService = "aks"
	ProviderKubernetes             = "kubernetes"
)

// ResourceType determines the type of the resource and the provider domain for the resource
type ResourceType struct {
	Type     string `bson:"type" json:"type"`
	Provider string `bson:"provider" json:"provider"`
}

func (r ResourceType) String() string {
	return fmt.Sprintf("Provider: %s, Type: %s", r.Provider, r.Type)
}

// ResourceIdentity represents the identity of a resource in the underlying system.
// - eg: For ARM this a Resource ID + API Version
// - eg: For Kubernetes this a GroupVersionKind + Namespace + Name
//
// This type supports safe serialization to/from JSON & BSON.
type ResourceIdentity struct {
	ResourceType *ResourceType `bson:"resourceType" json:"resourceType"`

	// A polymorphic payload. The fields in this data structure are determined by the provider field in the ResourceType
	Data interface{} `bson:"data" json:"data"`
}

// We just need custom Unmarshaling, default Marshaling is fine.
var _ json.Unmarshaler = (*ResourceIdentity)(nil)
var _ bson.Unmarshaler = (*ResourceIdentity)(nil)

// ARMIdentity uniquely identifies an ARM resource
type ARMIdentity struct {
	ID         string `bson:"id" json:"id"`
	APIVersion string `bson:"apiVersion" json:"apiVersion"`
}

// KubernetesIdentity uniquely identifies a Kubernetes resource
type KubernetesIdentity struct {
	Kind       string `bson:"kind" json:"kind"`
	APIVersion string `bson:"apiVersion" json:"apiVersion"`
	Name       string `bson:"name" json:"name"`
	Namespace  string `bson:"namespace" json:"namespace"`
}

// AADPodIdentityIdentity uniquely identifies a 'pod identity' psuedo-resource
type AADPodIdentityIdentity struct {
	AKSClusterName string `bson:"aksClusterName" json:"aksClusterName"`
	Name           string `bson:"name" json:"name"`
	Namespace      string `bson:"namespace" json:"namespace"`
}

func NewARMIdentity(resourceType *ResourceType, id string, apiVersion string) ResourceIdentity {
	return ResourceIdentity{
		ResourceType: resourceType,
		Data: ARMIdentity{
			ID:         id,
			APIVersion: apiVersion,
		},
	}
}

func NewKubernetesIdentity(resourceType *ResourceType, obj runtime.Object, objectMeta metav1.ObjectMeta) ResourceIdentity {
	return ResourceIdentity{
		ResourceType: resourceType,
		Data: KubernetesIdentity{
			Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
			APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Name:       objectMeta.Name,
			Namespace:  objectMeta.Namespace,
		},
	}
}

func (r ResourceIdentity) RequireARM() (string, string, error) {
	if r.ResourceType.Provider == ProviderAzure {
		data := r.Data.(ARMIdentity)
		return data.ID, data.APIVersion, nil
	}

	return "", "", fmt.Errorf("expected an %q provider, was %q", ProviderAzure, r.ResourceType.Provider)
}

func (r ResourceIdentity) RequireKubernetes() (schema.GroupVersionKind, string, string, error) {
	if r.ResourceType.Provider == ProviderKubernetes {
		data := r.Data.(KubernetesIdentity)
		return schema.FromAPIVersionAndKind(data.APIVersion, data.Kind), data.Namespace, data.Name, nil
	}

	return schema.GroupVersionKind{}, "", "", fmt.Errorf("expected an %q provider, was %q", ProviderKubernetes, r.ResourceType.Provider)
}

func (r ResourceIdentity) RequireAADPodIdentity() (string, string, string, error) {
	if r.ResourceType.Provider == ProviderAzureKubernetesService {
		data := r.Data.(AADPodIdentityIdentity)
		return data.AKSClusterName, data.Name, data.Namespace, nil
	}

	return "", "", "", fmt.Errorf("expected an %q provider, was %q", ProviderAzure, r.ResourceType.Provider)
}

func (r ResourceIdentity) IsSameResource(other ResourceIdentity) bool {
	if r.ResourceType.Provider != other.ResourceType.Provider {
		return false
	}

	switch r.ResourceType.Provider {
	case ProviderAzure:
		a, _ := r.Data.(ARMIdentity)
		b, _ := other.Data.(ARMIdentity)
		return a == b

	case ProviderKubernetes:
		a, _ := r.Data.(KubernetesIdentity)
		b, _ := other.Data.(KubernetesIdentity)
		return a == b

	case ProviderAzureKubernetesService:
		a, _ := r.Data.(AADPodIdentityIdentity)
		b, _ := other.Data.(AADPodIdentityIdentity)
		return a == b
	}

	// An identity without a valid kind is not the same as any resource.
	return false
}

// AsLogValues returns log values as key-value pairs from this ResourceIdentifier.
func (r ResourceIdentity) AsLogValues() []interface{} {
	if r.ResourceType == nil {
		return nil
	}
	switch r.ResourceType.Provider {
	case ProviderAzure:
		// We can't report an error here so this is best-effort.
		data := r.Data.(ARMIdentity)
		id, err := resources.Parse(data.ID)
		if err != nil {
			return []interface{}{radlogger.LogFieldResourceID, data.ID}
		}

		return []interface{}{
			radlogger.LogFieldResourceID, data.ID,
			radlogger.LogFieldSubscriptionID, id.FindScope(resources.SubscriptionsSegment),
			radlogger.LogFieldResourceGroup, id.FindScope(resources.ResourceGroupsSegment),
			radlogger.LogFieldResourceType, id.Type(),
			radlogger.LogFieldResourceName, id.QualifiedName(),
		}

	case ProviderKubernetes:
		data := r.Data.(KubernetesIdentity)
		return []interface{}{
			radlogger.LogFieldResourceName, data.Name,
			radlogger.LogFieldNamespace, data.Namespace,
			radlogger.LogFieldKind, data.Kind,
			radlogger.LogFieldResourceKind, resourcekinds.Kubernetes,
		}

	case ProviderAzureKubernetesService:
		return nil

	default:
		return nil
	}
}

func (r *ResourceIdentity) UnmarshalJSON(b []byte) error {
	type intermediate struct {
		ResourceType *ResourceType   `json:"resourceType"`
		Data         json.RawMessage `json:"data"`
	}

	data := intermediate{}
	err := json.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	r.ResourceType = data.ResourceType

	switch r.ResourceType.Provider {
	case ProviderAzure:
		identity := ARMIdentity{}
		err = json.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case ProviderKubernetes:
		identity := KubernetesIdentity{}
		err = json.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case ProviderAzureKubernetesService:
		identity := AADPodIdentityIdentity{}
		err = json.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil
	}

	err = json.Unmarshal(data.Data, &r.Data)
	if err != nil {
		return err
	}

	return nil
}

func (r *ResourceIdentity) UnmarshalBSON(b []byte) error {
	type intermediate struct {
		ResourceType *ResourceType `json:"resourceType"`
		Data         bson.Raw      `bson:"data"`
	}

	data := intermediate{}
	err := bson.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	r.ResourceType = data.ResourceType

	switch r.ResourceType.Provider {
	case ProviderAzure:
		identity := ARMIdentity{}
		err = bson.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case ProviderKubernetes:
		identity := KubernetesIdentity{}
		err = bson.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case ProviderAzureKubernetesService:
		identity := AADPodIdentityIdentity{}
		err = bson.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil
	default:
		return fmt.Errorf("unknown provider: %q", r.ResourceType.Provider)
	}
}
