// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcemodel

import (
	"encoding/json"
	"fmt"

	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"go.mongodb.org/mongo-driver/bson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Providers supported by Radius
// The RP will be able to support a resource only if the corresponding provider is configured with the RP
const (
	ProviderAzure      = "azure"
	ProviderKubernetes = "kubernetes"
)

// ResourceType determines the type of the resource and the provider domain for the resource
type ResourceType struct {
	Type     string `json:"type"`
	Provider string `json:"provider"`
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
	ResourceType *ResourceType `json:"resourceType"`
	// A polymorphic payload. The fields in this data structure are determined by the provider field in the ResourceType
	Data any `json:"data"`
}

// We just need custom Unmarshaling, default Marshaling is fine.
var _ json.Unmarshaler = (*ResourceIdentity)(nil)

// ARMIdentity uniquely identifies an ARM resource
type ARMIdentity struct {
	ID         string `json:"id"`
	APIVersion string `json:"apiVersion"`
}

// KubernetesIdentity uniquely identifies a Kubernetes resource
type KubernetesIdentity struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// AzureFederatedIdentity represents the federated identity for OIDC issuer.
type AzureFederatedIdentity struct {
	// Name represents the name of federeated identity.
	Name string `json:"name"`
	// Resource represents the associated identity resource.
	Resource string `json:"resource"`
	// OIDCIssuer represents the OIDC issuer.
	OIDCIssuer string `json:"oidcIssuer"`
	// Audience represents the client ID of Resource
	Audience string `json:"audience"`
	// Subejct represents the subject of Identity
	Subject string `json:"subject"`
}

func NewARMIdentity(resourceType *ResourceType, id string, apiVersion string) ResourceIdentity {
	return ResourceIdentity{
		ResourceType: &ResourceType{
			Type:     resourceType.Type,
			Provider: resourceType.Provider,
		},
		Data: ARMIdentity{
			ID:         id,
			APIVersion: apiVersion,
		},
	}
}

func NewKubernetesIdentity(resourceType *ResourceType, obj runtime.Object, objectMeta metav1.ObjectMeta) ResourceIdentity {
	return ResourceIdentity{
		ResourceType: &ResourceType{
			Type:     resourceType.Type,
			Provider: resourceType.Provider,
		},
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
		data, ok := r.Data.(ARMIdentity)
		if !ok {
			data = ARMIdentity{}
			if err := store.DecodeMap(r.Data, &data); err != nil {
				return "", "", err
			}
		}
		return data.ID, data.APIVersion, nil
	}

	return "", "", fmt.Errorf("expected an %q provider, was %q", ProviderAzure, r.ResourceType.Provider)
}

func (r ResourceIdentity) RequireKubernetes() (schema.GroupVersionKind, string, string, error) {
	if r.ResourceType.Provider == ProviderKubernetes {
		data, ok := r.Data.(KubernetesIdentity)
		if !ok {
			data = KubernetesIdentity{}
			if err := store.DecodeMap(r.Data, &data); err != nil {
				return schema.GroupVersionKind{}, "", "", err
			}
		}
		return schema.FromAPIVersionAndKind(data.APIVersion, data.Kind), data.Namespace, data.Name, nil
	}

	return schema.GroupVersionKind{}, "", "", fmt.Errorf("expected an %q provider, was %q", ProviderKubernetes, r.ResourceType.Provider)
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

	}
	// An identity without a valid kind is not the same as any resource.
	return false
}

// AsLogValues returns log values as key-value pairs from this ResourceIdentifier.
func (r ResourceIdentity) AsLogValues() []any {
	if r.ResourceType == nil {
		return nil
	}
	switch r.ResourceType.Provider {
	case ProviderAzure:
		// We can't report an error here so this is best-effort.
		data := r.Data.(ARMIdentity)
		id, err := resources.ParseResource(data.ID)
		if err != nil {
			return []any{logging.LogFieldResourceID, data.ID}
		}

		return []any{
			logging.LogFieldResourceID, data.ID,
			logging.LogFieldSubscriptionID, id.FindScope(resources.SubscriptionsSegment),
			logging.LogFieldResourceGroup, id.FindScope(resources.ResourceGroupsSegment),
			logging.LogFieldResourceType, id.Type(),
			logging.LogFieldResourceName, id.QualifiedName(),
		}

	case ProviderKubernetes:
		data := r.Data.(KubernetesIdentity)
		return []any{
			logging.LogFieldResourceName, data.Name,
			logging.LogFieldNamespace, data.Namespace,
			logging.LogFieldKind, data.Kind,
			logging.LogFieldResourceKind, resourcekinds.Kubernetes,
		}

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

	default:
		return fmt.Errorf("unknown provider: %q", r.ResourceType.Provider)
	}
}
