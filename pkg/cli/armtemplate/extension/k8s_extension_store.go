// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extension

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// K8sStore allows Kubernetes `existing` resources to be used in an ARM-JSON evaluator.
type K8sStore struct {
	log        logr.Logger
	client     dynamic.Interface
	restMapper meta.RESTMapper
}

var _ Store = &K8sStore{}

// NewK8sStore creates a new instance of K8sStore.
func NewK8sStore(log logr.Logger, client dynamic.Interface, restMapper meta.RESTMapper) *K8sStore {
	return &K8sStore{
		log:        log,
		client:     client,
		restMapper: restMapper,
	}
}

func (store *K8sStore) extractGroupVersionResourceName(ref string, version string) (schema.GroupVersionResource, string, error) {
	matches := regexp.MustCompile(`\.([^/.]+)/([^/]+)/([^/]+)$`).FindAllStringSubmatch(ref, -1)
	if len(matches) != 1 || len(matches[0]) != 4 {
		return schema.GroupVersionResource{}, "", fmt.Errorf("wrong reference format, expected: kubernetes.group/Kind/name, saw: %q", ref)
	}
	group, kind, name := matches[0][1], matches[0][2], matches[0][3]
	if group == "core" {
		group = ""
	}
	mapping, err := store.restMapper.RESTMapping(schema.GroupKind{Group: group, Kind: kind}, version)
	if err != nil {
		return schema.GroupVersionResource{}, "", err
	}
	return mapping.Resource, name, nil
}

// GetDeployedResource returns the K8s resource identified by the provided reference and the version string.
//
// The properties of this K8s resource are wrapped in a field called 'properties' as expected by the
// ARM-JSON evaluation logic.
func (store *K8sStore) GetDeployedResource(ref interface{}, version string) (map[string]interface{}, error) {
	gvr, name, err := store.extractGroupVersionResourceName(fmt.Sprintf("%v", ref), version)
	if err != nil {
		return nil, err
	}
	r, err := store.client.Resource(gvr).Namespace("default").Get(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if gvr.Group == "" && gvr.Version == "v1" && gvr.Resource == "secrets" {
		// Need to decode the secret data
		secretData := r.Object["data"].(map[string]interface{})
		data := make(map[string]interface{}, len(secretData))
		for k, v := range secretData {
			s, _ := v.(string)
			decoded, err := base64.StdEncoding.Strict().DecodeString(s)
			if err != nil {
				return nil, err
			}
			data[k] = string(decoded)
		}
		r.Object["data"] = data
	}
	return map[string]interface{}{
		// We have to nest all output inside "properties", since the compiled version
		// of references always access things under "properties" (e.g. ".properties.metadata.name")
		"properties": r.Object,
	}, nil
}
