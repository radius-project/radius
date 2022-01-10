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
	"strings"

	"github.com/project-radius/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/project-radius/radius/pkg/kubernetes/api/bicep/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func resourceGVK(resource Resource) (schema.GroupVersionKind, error) {
	// We name these like kubernetes.core/Secret or kubernetes.apps/Deployment.
	// So this code path is sensitive to how these are designed in Bicep.
	matches := regexp.MustCompile(`\.([^/.]+)/([^/]+)$`).FindAllStringSubmatch(resource.Type, -1)
	if len(matches) != 1 || len(matches[0]) != 3 {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid resource type, expect 'provider.group/Kind', saw %q", resource.Type)
	}
	// matches[0][0] is entire match, following that are the individual matched parts.
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
		return nil, fmt.Errorf("resource %s/%s lacks required property 'properties'", resource.Type, resource.Name)
	}
	r := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvk.GroupVersion().String(),
			"kind":       gvk.Kind,
		},
	}
	for k, v := range properties {
		if _, hasK := r.Object[k]; !hasK {
			r.Object[k] = v
		}
	}
	return r, nil
}

func scrapeSecrets(resource Resource) map[string]string {
	properties, ok := resource.Body["properties"].(map[string]interface{})
	if !ok {
		return nil
	}
	secrets, ok := properties["secrets"].(map[string]interface{})
	if !ok {
		return nil
	}
	if len(secrets) == 0 {
		return nil
	}
	result := make(map[string]string, len(secrets))
	for k, v := range secrets {
		if str, ok := v.(string); ok {
			result[k] = str
		}
	}
	delete(properties, "secrets")
	return result
}

func ConvertToK8sDeploymentTemplate(resource Resource, namespace string, parentName string) (*unstructured.Unstructured, error) {
	template, hasTemplate := resource.Body["template"].(map[string]interface{})
	if !hasTemplate {
		return nil, fmt.Errorf("resource %s/%s has no template", resource.Type, resource.Name)
	}
	parameters, _ := resource.Body["parameters"].(map[string]interface{})
	uns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": bicepv1alpha3.GroupVersion.String(),
			"kind":       bicepv1alpha3.DeploymentTemplateKind,
			"metadata": map[string]interface{}{
				"name":      parentName + "-" + resource.Name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"content":    template,
				"parameters": parameters,
			},
		},
	}
	return uns, nil
}

func ConvertToK8s(resource Resource, namespace string) (*unstructured.Unstructured, map[string]string, error) {
	annotations := map[string]string{}

	// K8s extension resources are not part of an application, so we can skip all the
	// application-related annotation logic.
	if resource.Provider != nil && resource.Provider.Name == "Kubernetes" {
		u, err := unwrapK8sUnstructured(resource)
		return u, nil, err
	}
	secrets := scrapeSecrets(resource)
	data, err := json.Marshal(resource)
	if err != nil {
		return nil, nil, err
	}
	applicationName, resourceName, resourceType := resource.GetRadiusResourceParts()

	if applicationName == "" {
		return nil, nil, errors.New("application name is empty")
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

	kind, ok := GetKindFromArmType(resourceType)
	if !ok {
		return nil, nil, fmt.Errorf("must have custom resource type mapping to arm type %s", resourceType)
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
	return uns, secrets, nil
}

var kindMap = map[string]string{
	"Application":               "Application",
	"Container":                 "Container",
	"dapr.io.PubSubTopic":       "DaprIOPubSubTopic",
	"dapr.io.StateStore":        "DaprIOStateStore",
	"dapr.io.InvokeHttpRoute":   "DaprIOInvokeHttpRoute",
	"mongo.com.MongoDatabase":   "MongoDatabase",
	"rabbitmq.com.MessageQueue": "RabbitMQMessageQueue",
	"redislabs.com.RedisCache":  "RedisCache",
	"HttpRoute":                 "HttpRoute",
	"GrpcRoute":                 "GrpcRoute",
	"Gateway":                   "Gateway",
}

// TODO this should be removed and instead we should use the CR definitions to know about the arm mapping

func GetKindFromArmType(armType string) (string, bool) {
	caseInsensitive := map[string]string{}
	for k, v := range kindMap {
		k := strings.ToLower(k)
		caseInsensitive[k] = v
	}

	res, ok := caseInsensitive[strings.ToLower(armType)]
	return res, ok
}

func GetSupportedTypes() map[string]string {
	return kindMap
}
