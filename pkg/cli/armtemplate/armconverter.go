// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/Azure/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func resourceGVK(resource Resource) (schema.GroupVersionKind, error) {
	matches := regexp.MustCompile(`\.([^/.]+)/([^/]+)$`).FindAllStringSubmatch(resource.Type, -1)
	if len(matches) != 1 || len(matches[0]) != 3 {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid resource type, expect 'provider.group/Kind', saw %q", resource.Type)
	}
	gvk := schema.GroupVersionKind{
		Group:   matches[0][1],
		Version: resource.APIVersion,
		Kind:    matches[0][2],
	}
	if gvk.Group == "core" {
		gvk.Group = ""
	}
	return gvk, nil
}

// unwrapK8sUnstructured unwraps a unstructured.Unstructured that was previous wrapped
// in a Resource's "properties" block.
func unwrapK8sUnstructured(resource Resource) (*unstructured.Unstructured, error) {
	gvk, err := resourceGVK(resource)
	if err != nil {
		return nil, err
	}
	// All wrapped K8s resource must have "properties", since at least
	// the metadata must always be there.
	properties, ok := resource.Body["properties"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("resource %s/%s has no property", resource.Type, resource.Name)
	}
	r := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvk.GroupVersion().String(),
			"kind":       gvk.Kind,
		},
	}
	// The "data" property of Secret needs to be unwrapped into `map[string][]byte`
	// to work well with the dynamic.Interface client.
	data, _ := properties["data"].(map[string]interface{})
	if len(data) > 0 && gvk.Group == "" && gvk.Kind == "Secret" && gvk.Version == "v1" {
		secretData := make(map[string][]byte, len(data))

		for k, v := range data {
			switch src := v.(type) {
			case string:
				secretData[k] = []byte(src)
			case []byte:
				secretData[k] = src
			}
		}
		r.Object["data"] = secretData
	}
	// Now fill in the rest of the properties.
	for k, v := range properties {
		if _, hasK := r.Object[k]; !hasK {
			r.Object[k] = v
		}
	}
	return r, nil
}

func ConvertToK8s(resource Resource, namespace string) (*unstructured.Unstructured, error) {
	annotations := map[string]string{}

	// K8s extension resources are not part of an application, so we can skip all the
	// application-related annotation logic.
	if resource.Provider != nil && resource.Provider.Name == "Kubernetes" {
		return unwrapK8sUnstructured(resource)
	}

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	applicationName, resourceName, resourceType := resource.GetRadiusResourceParts()

	if applicationName == "" {
		return nil, errors.New("application name is empty")
	}

	name := applicationName

	annotations[kubernetes.LabelRadiusApplication] = applicationName
	spec := map[string]interface{}{
		"template":    runtime.RawExtension{Raw: data},
		"application": applicationName,
	}

	if resourceType != "" && resourceName != "" {
		spec["resource"] = resourceName
		annotations[kubernetes.LabelRadiusResourceType] = resourceType
		annotations[kubernetes.LabelRadiusResource] = resourceName
		name = applicationName + "-" + resourceName
	}

	labels := kubernetes.MakeResourceCRDLabels(applicationName, resourceType, resourceName)

	kind := GetKindFromArmType(resourceType)
	if kind == "" {
		return nil, fmt.Errorf("must have custom resource type mapping to arm type %s", resourceType)
	}

	uns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "radius.dev/v1alpha3",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels":    labels,
			},

			"spec": spec,
		},
	}

	uns.SetAnnotations(annotations)
	return uns, nil
}

// TODO this should be removed and instead we should use the CR definitions to know about the arm mapping
func GetKindFromArmType(armType string) string {
	kindMap := map[string]string{
		"Application":                        "Application",
		"ContainerComponent":                 "ContainerComponent",
		"dapr.io.PubSubTopicComponent":       "DaprIOPubSubTopicComponent",
		"dapr.io.StateStoreComponent":        "DaprIOStateStoreComponent",
		"dapr.io.DaprHttpRoute":              "DaprIODaprHttpRoute",
		"mongodb.com.MongoDBComponent":       "MongoDBComponent",
		"rabbitmq.com.MessageQueueComponent": "RabbitMQComponent",
		"redislabs.com.RedisComponent":       "RedisComponent",
		"HttpRoute":                          "HttpRoute",
		"GrpcRoute":                          "GrpcRoute",
	}
	return kindMap[armType]
}
