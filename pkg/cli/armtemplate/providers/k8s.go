// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import (
	"context"
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
	namespace  string
	log        logr.Logger
	client     dynamic.Interface
	restMapper meta.RESTMapper
}

var _ Store = &K8sStore{}

// NewK8sStore creates a new instance of K8sStore.
func NewK8sStore(log logr.Logger, client dynamic.Interface, restMapper meta.RESTMapper) *K8sStore {
	return &K8sStore{
		// When rad-bicep supports passing through the namespace, we will need to plumb it in here.
		//
		// Note that since these are `external` resources that live outside an application, we can
		// not use the application namespace.
		namespace:  "default",
		log:        log,
		client:     client,
		restMapper: restMapper,
	}
}

func (store *K8sStore) extractGroupVersionResourceName(ref string, version string) (schema.GroupVersionResource, string, error) {
	// We name these like kubernetes.core/Secret/name or kubernetes.apps/Deployment/name
	// So this code path is sensitive to how these are designed in Bicep.
	matches := regexp.MustCompile(`\.([^/.]+)/([^/]+)/([^/]+)$`).FindAllStringSubmatch(ref, -1)
	if len(matches) != 1 || len(matches[0]) != 4 {
		return schema.GroupVersionResource{}, "", fmt.Errorf("wrong reference format, expected: kubernetes.group/Kind/name, saw: %q", ref)
	}
	// matches[0][0] is entire match, following that are the individual matched parts.
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
func (store *K8sStore) GetDeployedResource(ctx context.Context, ref string, version string) (map[string]interface{}, error) {
	gvr, name, err := store.extractGroupVersionResourceName(ref, version)
	if err != nil {
		return nil, err
	}
	r, err := store.client.Resource(gvr).Namespace(store.namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		// We have to nest all output inside "properties", since the compiled version
		// of references always access things under "properties" (e.g. ".properties.metadata.name")
		"properties": r.Object,
	}, nil
}
