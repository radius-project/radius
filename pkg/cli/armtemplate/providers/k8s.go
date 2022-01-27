// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const KubernetesProviderImport = "kubernetes"

var _ Provider = (*K8sProvider)(nil)

// K8sProvider allows Kubernetes `existing` resources to be used in an ARM-JSON evaluator.
type K8sProvider struct {
	namespace  string
	log        logr.Logger
	client     dynamic.Interface
	restMapper meta.RESTMapper
}

// NewK8sStore creates a new instance of K8sStore.
func NewK8sProvider(log logr.Logger, client dynamic.Interface, restMapper meta.RESTMapper) *K8sProvider {
	return &K8sProvider{
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

func (store *K8sProvider) extractGroupVersionResourceName(id string, version string) (schema.GroupVersionResource, schema.GroupVersionKind, string, error) {
	// We name these like kubernetes.core/Secret/name or kubernetes.apps/Deployment/name
	// So this code path is sensitive to how these are designed in Bicep.
	matches := regexp.MustCompile(`\.([^/.]+)/([^/]+)/([^/]+)$`).FindAllStringSubmatch(id, -1)
	if len(matches) != 1 || len(matches[0]) != 4 {
		return schema.GroupVersionResource{}, schema.GroupVersionKind{}, "", fmt.Errorf("wrong reference format, expected: kubernetes.group/Kind/name, saw: %q", id)
	}
	// matches[0][0] is entire match, following that are the individual matched parts.
	group, kind, name := matches[0][1], matches[0][2], matches[0][3]
	if group == "core" {
		group = ""
	}
	mapping, err := store.restMapper.RESTMapping(schema.GroupKind{Group: group, Kind: kind}, version)
	if err != nil {
		return schema.GroupVersionResource{}, schema.GroupVersionKind{}, "", err
	}

	return mapping.Resource, mapping.GroupVersionKind, name, nil
}

// GetDeployedResource returns the K8s resource identified by the provided reference and the version string.
//
// The properties of this K8s resource are wrapped in a field called 'properties' as expected by the
// ARM-JSON evaluation logic.
func (p *K8sProvider) GetDeployedResource(ctx context.Context, id string, version string) (map[string]interface{}, error) {
	gvr, _, name, err := p.extractGroupVersionResourceName(id, version)
	if err != nil {
		return nil, err
	}
	r, err := p.client.Resource(gvr).Namespace(p.namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		// We have to nest all output inside "properties", since the compiled version
		// of references always access things under "properties" (e.g. ".properties.metadata.name")
		"properties": r.Object,
	}, nil
}

func (p *K8sProvider) DeployResource(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error) {
	gvr, gvk, _, err := p.extractGroupVersionResourceName(id, version)
	if err != nil {
		return nil, err
	}

	// Unwrap the "properties" node
	obj := body["properties"].(map[string]interface{})
	obj["kind"] = gvk.Kind
	obj["apiVersion"] = gvk.GroupVersion().String()

	b, err := json.Marshal(&obj)
	if err != nil {
		return nil, err
	}

	name := obj["metadata"].(map[string]interface{})["name"].(string)
	r, err := p.client.Resource(gvr).Namespace(p.namespace).Patch(ctx, name, types.ApplyPatchType, b, v1.PatchOptions{FieldManager: "rad"})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		// We have to nest all output inside "properties", since the compiled version
		// of references always access things under "properties" (e.g. ".properties.metadata.name")
		"properties": r.Object,
	}, nil
}

func (p *K8sProvider) InvokeCustomAction(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error) {
	return nil, errors.New("the Kubernetes provider does not support custom actions")
}
