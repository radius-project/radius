// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcemodel

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"go.mongodb.org/mongo-driver/bson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Providers supported by Radius
// The RP will be able to support a resource only if the corresponding provider is configured with the RP
const (
	ProviderAzure      = "azure"
	ProviderAWS        = "aws"
	ProviderKubernetes = "kubernetes"

	// APIVersionUnknown encodes an "unknown" API version. Including API version in resource identity is
	// a design mistake because an API version is not part of the identity of a resource. We use this
	// value as a sentinel for the cases where we don't have a preferred API version.
	APIVersionUnknown = "unknown"
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

// UCPIdentity uniquely identifies a UCP resource
type UCPIdentity struct {
	ID string `json:"id"`
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

func NewUCPIdentity(resourceType *ResourceType, id string) ResourceIdentity {
	return ResourceIdentity{
		ResourceType: &ResourceType{
			Type:     resourceType.Type,
			Provider: resourceType.Provider,
		},
		Data: UCPIdentity{
			ID: id,
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

// GetID constructs a UCP resource ID from the ResourceIdentity.
func (r ResourceIdentity) GetID() string {
	switch r.ResourceType.Provider {
	case ProviderAzure:
		id, _, _ := r.RequireARM()
		return id
	case ProviderAWS:
		id, _ := r.RequireAWS()
		return id
	case ProviderKubernetes:
		gvk, namespace, name, _ := r.RequireKubernetes()
		group := gvk.Group
		if group == "" {
			group = "core"
		}
		if namespace == "" {
			return fmt.Sprintf("/planes/kubernetes/local/providers/%s/%s/%s", group, gvk.Kind, name)
		} else {
			return fmt.Sprintf("/planes/kubernetes/local/namespaces/%s/providers/%s/%s/%s", namespace, group, gvk.Kind, name)
		}
	default:
		return ""
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

func (r ResourceIdentity) RequireAWS() (string, error) {
	if r.ResourceType.Provider == ProviderAWS {
		data, ok := r.Data.(UCPIdentity)
		if !ok {
			data = UCPIdentity{}
			if err := store.DecodeMap(r.Data, &data); err != nil {
				return "", err
			}
		}
		return data.ID, nil
	}

	return "", fmt.Errorf("expected an %q provider, was %q", ProviderAWS, r.ResourceType.Provider)
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

	case ProviderAWS:
		a, _ := r.Data.(UCPIdentity)
		b, _ := other.Data.(UCPIdentity)
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
func (r ResourceIdentity) AsLogValues() []interface{} {
	if r.ResourceType == nil {
		return nil
	}
	switch r.ResourceType.Provider {
	case ProviderAzure:
		// We can't report an error here so this is best-effort.
		data := r.Data.(ARMIdentity)
		id, err := resources.ParseResource(data.ID)
		if err != nil {
			return []any{ucplog.LogFieldResourceID, data.ID}
		}

		return []interface{}{
			logging.LogFieldResourceID, data.ID,
			logging.LogFieldSubscriptionID, id.FindScope(resources.SubscriptionsSegment),
			logging.LogFieldResourceGroup, id.FindScope(resources.ResourceGroupsSegment),
			logging.LogFieldResourceType, id.Type(),
			logging.LogFieldResourceName, id.QualifiedName(),
		}

	case ProviderAWS:
		// We can't report an error here so this is best-effort.
		data := r.Data.(UCPIdentity)
		id, err := resources.ParseResource(data.ID)
		if err != nil {
			return []any{ucplog.LogFieldResourceID, data.ID}
		}

		return []any{
			logging.LogFieldResourceID, data.ID,
			logging.LogFieldResourceType, id.Type(),
			logging.LogFieldResourceName, id.QualifiedName(),
		}

	case ProviderKubernetes:
		data := r.Data.(KubernetesIdentity)
		return []interface{}{
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

	case ProviderAWS:
		identity := UCPIdentity{}
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

	case ProviderAWS:
		identity := UCPIdentity{}
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

// FromUCPID translates a UCP resource ID into a ResourceIdentity.
//
// TODO: This is transitional while we're refactoring to get rid of ResourceIdentity. UCP resource IDs are a more
// complete and flexible way of identitying resources.
func FromUCPID(id resources.ID, preferredAPIVersion string) ResourceIdentity {
	// Blank resource id => blank identity
	if len(id.ScopeSegments()) == 0 {
		return ResourceIdentity{}
	}

	firstScope := id.ScopeSegments()[0].Type
	if preferredAPIVersion == "" {
		preferredAPIVersion = APIVersionUnknown
	}

	// If this starts with a subscription ID then it's an Azure resource
	//
	// case: /subscriptions/.../resourceGroups/.../......
	if strings.EqualFold(resources.SubscriptionsSegment, firstScope) {
		return NewARMIdentity(&ResourceType{Type: id.Type(), Provider: ProviderAzure}, id.String(), preferredAPIVersion)
	}

	// case: /planes/azure/azurecloud/subscriptions/.../resourceGroups/.../......
	if strings.EqualFold("azure", firstScope) {
		return NewARMIdentity(&ResourceType{Type: id.Type(), Provider: ProviderAzure}, id.String(), preferredAPIVersion)
	}

	// case: /planes/aws/aws/accounts/.../regions/.../......
	if strings.EqualFold("aws", firstScope) {
		return NewUCPIdentity(&ResourceType{Type: id.Type(), Provider: ProviderAWS}, id.String())
	}

	// case: /planes/kubernetes/local/namespaces/.../......
	if strings.EqualFold("kubernetes", firstScope) {
		// Kubernetes has some quirks because API groups were added after the initial release.
		// We encode the "unnamed" group as "core".
		group, kind, _ := strings.Cut(id.Type(), "/")
		resourceType := id.Type()
		apiVersion := group + "/" + preferredAPIVersion
		if strings.EqualFold(group, "core") {
			resourceType = kind
			apiVersion = preferredAPIVersion
		}

		return ResourceIdentity{
			ResourceType: &ResourceType{
				Type:     resourceType,
				Provider: ProviderKubernetes,
			},
			Data: KubernetesIdentity{
				Kind:       kind,
				APIVersion: apiVersion,
				Namespace:  id.FindScope("namespaces"),
				Name:       id.Name(),
			},
		}
	}

	return ResourceIdentity{}
}
