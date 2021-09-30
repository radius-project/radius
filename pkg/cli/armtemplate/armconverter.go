// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertToK8s(resource Resource, namespace string) (*unstructured.Unstructured, error) {
	annotations := map[string]string{}

	// Compute annotations to capture the name segments
	nameParts := strings.Split(resource.Name, "/")

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
		name = applicationName + "." + resourceName
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
