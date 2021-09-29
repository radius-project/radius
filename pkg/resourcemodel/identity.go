// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resourcemodel

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radlogger"
	"go.mongodb.org/mongo-driver/bson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ResourceIdentityKind string

// OutputResource Types
const (
	IdentityKindARM            ResourceIdentityKind = "arm"
	IdentityKindKubernetes     ResourceIdentityKind = "kubernetes"
	IdentityKindAADPodIdentity ResourceIdentityKind = "aadpodidentity"

	// NOTE: you should only add new types here if you are adding a new **SYSTEM** for
	// Radius to interface with OR if you are adding a new *pseudo-resource* type.
	//
	// Adding a new kind of identity also requires updating the seralization code.
)

// ResourceIdentity represents the identity of a resource in the underlying system.
// - eg: For ARM this a Resource ID + API Version
// - eg: For Kubernetes this a GroupVersionKind + Namespace + Name
//
// This type supports safe serialization to/from JSON & BSON.
type ResourceIdentity struct {
	Kind ResourceIdentityKind `bson:"kind" json:"kind"`

	// A polymorphic payload.
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

func NewARMIdentity(id string, apiVersion string) ResourceIdentity {
	return ResourceIdentity{
		Kind: IdentityKindARM,
		Data: ARMIdentity{
			ID:         id,
			APIVersion: apiVersion,
		},
	}
}

func NewKubernetesIdentity(obj runtime.Object, objectMeta metav1.ObjectMeta) ResourceIdentity {
	return ResourceIdentity{
		Kind: IdentityKindKubernetes,
		Data: KubernetesIdentity{
			Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
			APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Name:       objectMeta.Name,
			Namespace:  objectMeta.Namespace,
		},
	}
}

func (r ResourceIdentity) IsSameResource(other ResourceIdentity) bool {
	if r.Kind != other.Kind {
		return false
	}

	switch r.Kind {
	case IdentityKindARM:
		a, _ := r.Data.(ARMIdentity)
		b, _ := other.Data.(ARMIdentity)
		return a == b

	case IdentityKindKubernetes:
		a, _ := r.Data.(KubernetesIdentity)
		b, _ := other.Data.(KubernetesIdentity)
		return a == b

	case IdentityKindAADPodIdentity:
		a, _ := r.Data.(AADPodIdentityIdentity)
		b, _ := other.Data.(AADPodIdentityIdentity)
		return a == b
	}

	// An identity without a valid kind is not the same as any resource.
	return false
}

// AsLogValues returns log values as key-value pairs from this ResourceIdentifier.
func (r ResourceIdentity) AsLogValues() []interface{} {
	switch r.Kind {
	case IdentityKindARM:
		// We can't report an error here so this is best-effort.
		data := r.Data.(ARMIdentity)
		id, err := azresources.Parse(data.ID)
		if err != nil {
			return []interface{}{radlogger.LogFieldResourceID, data.ID}
		}

		return []interface{}{
			radlogger.LogFieldResourceID, data.ID,
			radlogger.LogFieldSubscriptionID, id.SubscriptionID,
			radlogger.LogFieldResourceGroup, id.ResourceGroup,
			radlogger.LogFieldResourceType, id.Type(),
			radlogger.LogFieldResourceName, id.QualifiedName(),
		}

	case IdentityKindKubernetes:
		return nil

	case IdentityKindAADPodIdentity:
		return nil

	default:
		return nil
	}
}

func (r *ResourceIdentity) UnmarshalJSON(b []byte) error {
	type intermediate struct {
		Kind ResourceIdentityKind `json:"kind"`
		Data json.RawMessage      `json:"data"`
	}

	data := intermediate{}
	err := json.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	r.Kind = data.Kind

	switch r.Kind {
	case IdentityKindARM:
		identity := ARMIdentity{}
		err = json.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case IdentityKindKubernetes:
		identity := KubernetesIdentity{}
		err = json.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case IdentityKindAADPodIdentity:
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
		Kind ResourceIdentityKind `bson:"kind"`
		Data bson.Raw             `bson:"data"`
	}

	data := intermediate{}
	err := bson.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	r.Kind = data.Kind

	switch r.Kind {
	case IdentityKindARM:
		identity := ARMIdentity{}
		err = bson.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case IdentityKindKubernetes:
		identity := KubernetesIdentity{}
		err = bson.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	case IdentityKindAADPodIdentity:
		identity := AADPodIdentityIdentity{}
		err = bson.Unmarshal(data.Data, &identity)
		if err != nil {
			return err
		}
		r.Data = identity
		return nil

	default:
		return fmt.Errorf("unknown identity kind: %q", r.Kind)
	}
}
